package applicationlog

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// IsRefusal reports whether err is a refusal of the caller's input or of an id (as opposed to a failure of the database): the routes
// answer these with a 400 and the AI tools with the reason, so the model can correct the call.
func IsRefusal(err error) bool {
	for _, e := range []error{ErrUnknownProfile, ErrUnknownJob, ErrUnknownMessage, ErrUnknownRecruiter, ErrInvalidDirection, ErrInvalidChannel,
		ErrInvalidDate, ErrTimeWithoutTime, ErrInvalidURL, ErrTooLong} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// --- AI tools (plan/ai/tools/career/step-176-application-protocol.md, step 2) ---

func errResult(action string, err error) *mcp.CallToolResult {
	switch {
	case errors.Is(err, ErrUnknownProfile):
		return db.ErrResult("unknown profile id (see list_profiles)")
	case errors.Is(err, ErrUnknownJob):
		return db.ErrResult("unknown job id (see list_jobs)")
	case errors.Is(err, ErrUnknownMessage):
		return db.ErrResult("unknown message id (see list_application_messages)")
	case errors.Is(err, ErrUnknownRecruiter):
		return db.ErrResult("unknown recruiter id (see list_recruiters)")
	case IsRefusal(err):
		return db.ErrResult(err.Error())
	}
	return db.ErrResult(fmt.Sprintf("failed to %s: %v", action, err))
}

type listArgs struct {
	ProfileID string `json:"profileId" jsonschema:"the profile the application belongs to (from list_profiles)"`
	JobID     string `json:"jobId" jsonschema:"the job the application is for (from list_jobs)"`
}

// RegisterListApplicationMessages registers list_application_messages.
func RegisterListApplicationMessages(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_application_messages",
		Description: "List the application protocol of one profile and one job: the dated log of the communication (emails, platform messages, SMS, WhatsApp, calls, meetings), oldest first. " +
			"Use it to answer questions such as when the person last heard from a recruiter. Each entry has its id, direction (sent, received or note), channel, date, subject and text.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" || args.JobID == "" {
			return db.ErrResult("profileId and jobId are required"), nil, nil
		}
		list, err := List(args.ProfileID, args.JobID)
		if err != nil {
			return errResult("list application messages", err), nil, nil
		}
		return db.JSONResult(map[string]any{"messages": list})
	})
}

type addArgs struct {
	ProfileID          string `json:"profileId" jsonschema:"the profile the application belongs to (from list_profiles)"`
	JobID              string `json:"jobId" jsonschema:"the job the application is for (from list_jobs)"`
	Direction          string `json:"direction" jsonschema:"sent (the person wrote), received (the other side wrote) or note (not a message: a call that was not answered, an agreement)"`
	Channel            string `json:"channel" jsonschema:"email, platform (a job platform or LinkedIn), sms, whatsapp, phone, video, in_person, letter or other"`
	OccurredAt         string `json:"occurredAt" jsonschema:"when it happened: a date (2026-11-05) or a date and time (2026-11-05T14:30 or with a time zone); a future date is fine for a planned meeting"`
	HasTime            *bool  `json:"hasTime,omitempty" jsonschema:"false when only the day is known; omit when occurredAt has a time"`
	ChannelLabel       string `json:"channelLabel,omitempty" jsonschema:"the name of the platform or app, e.g. LinkedIn or Signal"`
	Subject            string `json:"subject,omitempty" jsonschema:"the subject line or a short title"`
	Body               string `json:"body,omitempty" jsonschema:"the text of the message or what was said"`
	RecruiterID        string `json:"recruiterId,omitempty" jsonschema:"the recruiter on the other side, when known (from list_recruiters)"`
	CounterpartName    string `json:"counterpartName,omitempty" jsonschema:"who the other side is, when not a stored recruiter"`
	CounterpartContact string `json:"counterpartContact,omitempty" jsonschema:"their address, number or handle"`
	CVDocumentID       string `json:"cvDocumentId,omitempty" jsonschema:"the id of the CV that was sent, when one was"`
	ExternalURL        string `json:"externalUrl,omitempty" jsonschema:"a link to the thread, starting with http:// or https://"`
}

// RegisterAddApplicationMessage registers add_application_message.
func RegisterAddApplicationMessage(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "add_application_message",
		Description: "Write one entry into the application protocol of a profile and a job: a message sent or received, or a note. direction, channel and occurredAt are required. " +
			"Only record what the person told you; never invent a message or a date. Returns the entry's id.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" || args.JobID == "" {
			return db.ErrResult("profileId and jobId are required"), nil, nil
		}
		m, err := Add(args.ProfileID, args.JobID, Input{Direction: args.Direction, Channel: args.Channel, ChannelLabel: args.ChannelLabel, OccurredAt: args.OccurredAt,
			HasTime: args.HasTime, Subject: args.Subject, Body: args.Body, RecruiterID: args.RecruiterID, CounterpartName: args.CounterpartName,
			CounterpartContact: args.CounterpartContact, CVDocumentID: args.CVDocumentID, ExternalURL: args.ExternalURL})
		if err != nil {
			return errResult("add application message", err), nil, nil
		}
		return db.JSONResult(map[string]string{"id": m.ID})
	})
}

type updateArgs struct {
	ID                 string  `json:"id" jsonschema:"the entry's id (from list_application_messages)"`
	Direction          *string `json:"direction,omitempty" jsonschema:"sent, received or note"`
	Channel            *string `json:"channel,omitempty" jsonschema:"email, platform, sms, whatsapp, phone, video, in_person, letter or other"`
	OccurredAt         *string `json:"occurredAt,omitempty" jsonschema:"a date (2026-11-05) or a date and time (2026-11-05T14:30)"`
	HasTime            *bool   `json:"hasTime,omitempty" jsonschema:"false keeps only the day"`
	ChannelLabel       *string `json:"channelLabel,omitempty" jsonschema:"the name of the platform or app"`
	Subject            *string `json:"subject,omitempty" jsonschema:"the subject or a short title"`
	Body               *string `json:"body,omitempty" jsonschema:"the text"`
	RecruiterID        *string `json:"recruiterId,omitempty" jsonschema:"the recruiter; an empty text clears it"`
	CounterpartName    *string `json:"counterpartName,omitempty" jsonschema:"who the other side is"`
	CounterpartContact *string `json:"counterpartContact,omitempty" jsonschema:"their address, number or handle"`
	CVDocumentID       *string `json:"cvDocumentId,omitempty" jsonschema:"the CV that was sent; an empty text clears it"`
	ExternalURL        *string `json:"externalUrl,omitempty" jsonschema:"a link to the thread; an empty text clears it"`
}

// RegisterUpdateApplicationMessage registers update_application_message.
func RegisterUpdateApplicationMessage(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_application_message",
		Description: "Change an entry of the application protocol: only the fields provided change, and the whole entry must stay valid.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if _, err := Update(args.ID, Patch{Direction: args.Direction, Channel: args.Channel, ChannelLabel: args.ChannelLabel, OccurredAt: args.OccurredAt,
			HasTime: args.HasTime, Subject: args.Subject, Body: args.Body, RecruiterID: args.RecruiterID, CounterpartName: args.CounterpartName,
			CounterpartContact: args.CounterpartContact, CVDocumentID: args.CVDocumentID, ExternalURL: args.ExternalURL}); err != nil {
			return errResult("update application message", err), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type removeArgs struct {
	ID string `json:"id" jsonschema:"the entry's id (from list_application_messages)"`
}

// RegisterRemoveApplicationMessage registers remove_application_message.
func RegisterRemoveApplicationMessage(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_application_message",
		Description: "Remove one entry from the application protocol. Only when the person asks for it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args removeArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if err := Delete(args.ID); err != nil {
			return errResult("remove application message", err), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "removed"}}}, nil, nil
	})
}
