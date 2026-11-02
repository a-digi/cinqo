// Package jobrecruiters keeps the mapping between jobs and recruiters (plan/ai/tools/career/step-168-recruiters-on-jobs.md): a job
// can have several recruiters and a recruiter several jobs. Both live in jobs.db, so the table job_recruiters references them with
// foreign keys and deleting a job or a recruiter removes the rows by itself (a company being deleted never does).
package jobrecruiters

import (
	"database/sql"
	"strings"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/recruiters"
)

// MappedRecruiter is a recruiter mapped to one job, with what a list needs to show it.
type MappedRecruiter struct {
	ID        string `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	// CompanyID/CompanyName: the company linked first, or "" (older callers); Companies: all of them.
	CompanyID   string               `json:"companyId"`
	CompanyName string               `json:"companyName"`
	Companies   []recruiters.Company `json:"companies"`
	MappedAt    string               `json:"mappedAt"`
}

// MappedJob is a job mapped to one recruiter.
type MappedJob struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Company   string `json:"company,omitempty"`
	CompanyID string `json:"companyId,omitempty"`
	Location  string `json:"location,omitempty"`
	MappedAt  string `json:"mappedAt"`
}

func exists(query, id string) (bool, error) {
	var one int
	err := db.JobsDB.QueryRow(query, id).Scan(&one)
	switch {
	case err == sql.ErrNoRows:
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

// Map puts a recruiter on a job. created is false when the mapping already existed (nothing changes then, not even its time).
// jobs.ErrUnknownJob and recruiters.ErrUnknownRecruiter say which of the two is missing; nothing is written in that case.
func Map(jobID, recruiterID string) (created bool, err error) {
	jobID, recruiterID = strings.TrimSpace(jobID), strings.TrimSpace(recruiterID)
	if ok, err := exists(`SELECT 1 FROM jobs WHERE id = ?`, jobID); err != nil {
		return false, err
	} else if !ok {
		return false, jobs.ErrUnknownJob
	}
	if ok, err := exists(`SELECT 1 FROM recruiters WHERE id = ?`, recruiterID); err != nil {
		return false, err
	} else if !ok {
		return false, recruiters.ErrUnknownRecruiter
	}
	res, err := db.JobsDB.Exec(
		`INSERT INTO job_recruiters (job_id, recruiter_id, created_at) VALUES (?, ?, datetime('now')) ON CONFLICT(job_id, recruiter_id) DO NOTHING`,
		jobID, recruiterID,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// Unmap takes a recruiter off a job; it is not an error when they were not mapped (or when either id does not exist).
func Unmap(jobID, recruiterID string) error {
	_, err := db.JobsDB.Exec(`DELETE FROM job_recruiters WHERE job_id = ? AND recruiter_id = ?`, strings.TrimSpace(jobID), strings.TrimSpace(recruiterID))
	return err
}

// ForJob lists the recruiters mapped to a job, the newest mapping first, with the companies they work for (plan step-169).
// CompanyID/CompanyName are the company linked first, or empty, so a caller written for one company keeps working. An unknown
// job has none.
func ForJob(jobID string) ([]MappedRecruiter, error) {
	rows, err := db.JobsDB.Query(
		`SELECT r.id, r.first_name, r.last_name, r.email, jr.created_at
		 FROM job_recruiters jr
		 JOIN recruiters r ON r.id = jr.recruiter_id
		 WHERE jr.job_id = ?
		 ORDER BY jr.created_at DESC, jr.rowid DESC`, strings.TrimSpace(jobID))
	if err != nil {
		return nil, err
	}
	out := []MappedRecruiter{}
	for rows.Next() {
		m := MappedRecruiter{Companies: []recruiters.Company{}}
		if err := rows.Scan(&m.ID, &m.FirstName, &m.LastName, &m.Email, &m.MappedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close() // one connection only: the companies are read after this result set is closed
	for i := range out {
		links, err := db.JobsDB.Query(
			`SELECT c.id, c.name FROM recruiter_companies rc JOIN companies c ON c.id = rc.company_id
			 WHERE rc.recruiter_id = ? ORDER BY rc.created_at ASC, rc.rowid ASC`, out[i].ID)
		if err != nil {
			return nil, err
		}
		for links.Next() {
			var c recruiters.Company
			if err := links.Scan(&c.ID, &c.Name); err != nil {
				links.Close()
				return nil, err
			}
			out[i].Companies = append(out[i].Companies, c)
		}
		if err := links.Err(); err != nil {
			links.Close()
			return nil, err
		}
		links.Close()
		if len(out[i].Companies) > 0 {
			out[i].CompanyID, out[i].CompanyName = out[i].Companies[0].ID, out[i].Companies[0].Name
		}
	}
	return out, nil
}

// ForRecruiter lists the jobs a recruiter is mapped to, the newest mapping first. An unknown recruiter has none.
func ForRecruiter(recruiterID string) ([]MappedJob, error) {
	rows, err := db.JobsDB.Query(
		`SELECT j.id, j.title, j.company, j.company_id, j.location, jr.created_at
		 FROM job_recruiters jr
		 JOIN jobs j ON j.id = jr.job_id
		 WHERE jr.recruiter_id = ?
		 ORDER BY jr.created_at DESC, jr.rowid DESC`, strings.TrimSpace(recruiterID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MappedJob{}
	for rows.Next() {
		var m MappedJob
		var company, companyID, location sql.NullString
		if err := rows.Scan(&m.ID, &m.Title, &company, &companyID, &location, &m.MappedAt); err != nil {
			return nil, err
		}
		m.Company, m.CompanyID, m.Location = company.String, companyID.String, location.String
		out = append(out, m)
	}
	return out, rows.Err()
}

// RequireJob returns jobs.ErrUnknownJob when no job has this id.
func RequireJob(jobID string) error {
	if ok, err := exists(`SELECT 1 FROM jobs WHERE id = ?`, strings.TrimSpace(jobID)); err != nil {
		return err
	} else if !ok {
		return jobs.ErrUnknownJob
	}
	return nil
}

// RequireRecruiter returns recruiters.ErrUnknownRecruiter when no recruiter has this id.
func RequireRecruiter(recruiterID string) error {
	if ok, err := exists(`SELECT 1 FROM recruiters WHERE id = ?`, strings.TrimSpace(recruiterID)); err != nil {
		return err
	} else if !ok {
		return recruiters.ErrUnknownRecruiter
	}
	return nil
}
