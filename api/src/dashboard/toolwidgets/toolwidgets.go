// Package toolwidgets is the dashboard Source for installed tools: it turns the
// widgets a tool declared in its manifest (cached at install) into providers that
// fetch their data from the tool's own backend. See
// plan/ai/dashboard/home-dashboard.md.
package toolwidgets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"syscall"

	"github.com/a-digi/cinqo/src/dashboard"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// PortFunc reports the loopback port a tool's backend listens on, and whether it
// is running (tool/manager.Port).
type PortFunc func(toolID string) (port int, running bool)

// Source lists the enabled tools' widgets, asked afresh on every request so an
// install, update, enable, disable or uninstall shows up without a restart.
type Source struct {
	db     *sql.DB
	port   PortFunc
	client *http.Client
	logf   func(format string, args ...any)
}

func NewSource(db *sql.DB, port PortFunc) *Source {
	return &Source{
		db:   db,
		port: port,
		// Redirects are never followed: a tool answers its own route, and a
		// redirect would be a way to make core call somewhere else.
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		logf:   func(string, ...any) {},
	}
}

func (s *Source) WithLogger(logf func(format string, args ...any)) *Source {
	s.logf = logf
	return s
}

func (s *Source) Providers() []dashboard.Provider {
	widgets, err := tool_query.NewToolDashboardWidgetQueryRepo(s.db).FindAllEnabled()
	if err != nil {
		s.logf("dashboard: listing tool widgets failed: %v", err)
		return nil
	}
	out := make([]dashboard.Provider, 0, len(widgets))
	for _, w := range widgets {
		def := dashboard.Definition{
			Provider:       w.ToolSlug,
			Key:            w.Key,
			Title:          w.Title,
			Description:    w.Description,
			Kind:           dashboard.Kind(w.Kind),
			Size:           dashboard.Size{W: w.Width, H: w.Height},
			RefreshSeconds: w.RefreshSeconds,
			// The widget's scope is its data route's, so an empty scope here
			// (a route that somehow has none) must not mean "everyone".
			RequiredScope: w.RequiredScope,
		}
		if def.RequiredScope == "" {
			def.RequiredScope = dashboard.AdminScope
		}
		// A row that no longer satisfies the rules (a hand-edited database, a
		// range tightened in a later release) is skipped, never served.
		if err := def.Validate(); err != nil || w.SchemaVersion != dashboard.SchemaVersion {
			s.logf("dashboard: skipping widget %s.%s: invalid definition", w.ToolSlug, w.Key)
			continue
		}
		out = append(out, &provider{source: s, toolID: w.ToolID, path: w.DataPath, def: def})
	}
	return out
}

type provider struct {
	source *Source
	toolID string
	path   string
	def    dashboard.Definition
}

func (p *provider) Definition() dashboard.Definition { return p.def }

// Data does GET http://127.0.0.1:<tool port>/<data path> with the caller's own
// bearer token. The address is built from the tool manager's port, never from
// anything the manifest says. The body is read through a hard cap and handed to
// the engine, which validates it.
func (p *provider) Data(ctx context.Context, caller dashboard.Caller) ([]byte, error) {
	port, running := p.source.port(p.toolID)
	if !running {
		return nil, dashboard.ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/"+p.path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if caller.Token != "" {
		req.Header.Set("Authorization", "Bearer "+caller.Token)
	}

	resp, err := p.source.client.Do(req)
	if err != nil {
		if isConnectionRefused(err) {
			return nil, dashboard.ErrUnavailable
		}
		return nil, fmt.Errorf("call tool: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, dashboard.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("tool answered %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, dashboard.MaxPayloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read tool response: %w", err)
	}
	if len(body) > dashboard.MaxPayloadBytes {
		return nil, fmt.Errorf("tool response is over the %d byte limit", dashboard.MaxPayloadBytes)
	}
	return body, nil
}

func isConnectionRefused(err error) bool {
	var op *net.OpError
	return errors.As(err, &op) && errors.Is(op.Err, syscall.ECONNREFUSED)
}
