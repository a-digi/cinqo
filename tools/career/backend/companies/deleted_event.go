package companies

import (
	"career-tool-backend/domainevent"
)

// PublishCompanyDeleted announces "career.company.deleted" for a company whose row was really removed (step 139), with
// the id and the name the company had. Best effort like every domain event, and synchronous: domainevent.Publish's own
// short timeout bounds it. Callers send it only after the delete succeeded and affected a row.
func PublishCompanyDeleted(id, name string) {
	domainevent.Publish("career.company.deleted", map[string]string{"company_id": id, "name": name})
}
