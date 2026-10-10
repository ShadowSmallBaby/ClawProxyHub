package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/action"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func WithExtensions(m *extension.Manager, registry *action.Registry) Option {
	return func(s *Server) { s.extensions = m; s.actions = registry }
}
func WithSystemComponents(query func() []extension.SystemComponent) Option {
	return func(s *Server) { s.systemComponents = query }
}
func (s *Server) routeExtensions(r authed) {
	if s.extensions == nil || s.actions == nil {
		return
	}
	r.h("GET /admin/extensions", s.listExtensions)
	r.h("GET /admin/extensions/catalog", s.extensionCatalog)
	r.h("GET /admin/extensions/marketplace", s.extensionMarketplace)
	r.h("POST /admin/extensions/inspect-market", s.inspectMarketExtension)
	r.h("GET /admin/extension-trust", s.extensionTrust)
	r.h("PUT /admin/extension-trust/{code}", s.putExtensionTrust)
	r.h("DELETE /admin/extension-trust/{code}", s.deleteExtensionTrust)
	r.h("POST /admin/extensions/install-mounted", s.installMountedExtension)
	r.h("POST /admin/extensions/install", s.installExtension)
	r.h("POST /admin/extensions/inspect", s.installExtension)
	r.h("PUT /admin/extensions/{id}/enabled", s.enableExtension)
	r.h("GET /admin/extensions/{id}/settings", s.getExtensionSettings)
	r.h("PUT /admin/extensions/{id}/settings", s.putExtensionSettings)
	r.h("DELETE /admin/extensions/{id}", s.uninstallExtension)
	r.h("DELETE /admin/extensions/{id}/cache", s.cleanExtensionCache)
	r.h("DELETE /admin/extensions/{id}/data", s.cleanExtensionData)
	r.h("GET /admin/actions", s.listActions)
	r.h("POST /admin/actions/{id}", s.invokeAction)
}
func (s *Server) principal(r *http.Request) action.Principal {
	if c, ok := r.Context().Value(ctxKeyClaims{}).(*jwtClaims); ok {
		p := action.Principal{Subject: c.Sub, Role: c.Role}
		if c.Scoped {
			p.Scopes = append([]string{}, c.Scopes...)
		}
		return p
	}
	return action.Principal{Subject: s.lookupUsername(r), Role: roleOf(r)}
}
func (s *Server) listActions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"actions": s.actions.List(s.principal(r)), "generation": s.actions.Generation()})
}
func (s *Server) invokeAction(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, action.MaxJSONBytes+1))
	if err != nil || len(raw) > action.MaxJSONBytes {
		writeJSON(w, 400, map[string]string{"error": "invalid action payload"})
		return
	}
	result, err := s.actions.Invoke(r.Context(), s.principal(r), r.PathValue("id"), raw)
	if err != nil {
		code := 400
		if errors.Is(err, action.ErrForbidden) {
			code = 403
		}
		if errors.Is(err, action.ErrUnavailable) {
			code = 404
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) listExtensions(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	components := []extension.SystemComponent{}
	if s.systemComponents != nil {
		components = s.systemComponents()
	}
	writeJSON(w, 200, map[string]any{"extensions": s.extensions.ListFor(r.Context()), "system_components": components,
		"runtime_management": s.extensions.RuntimeManagement(r.Context())})
}
func (s *Server) extensionCatalog(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	writeJSON(w, 200, map[string]any{"packages": s.extensions.CatalogFor(r.Context())})
}

func (s *Server) extensionTrust(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	writeJSON(w, 200, map[string]any{"identities": s.extensions.TrustEntries()})
}

func (s *Server) putExtensionTrust(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var config spec.TrustKey
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		extensionReply(w, err)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		extensionReply(w, errors.New("expected one signing identity configuration"))
		return
	}
	extensionReply(w, s.extensions.UpdateTrust(r.PathValue("code"), &config))
}

func (s *Server) deleteExtensionTrust(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	extensionReply(w, s.extensions.UpdateTrust(r.PathValue("code"), nil))
}
func (s *Server) installMountedExtension(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var in struct {
		SHA256 string   `json:"sha256"`
		Grants []string `json:"grants"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in); err != nil {
		extensionReply(w, err)
		return
	}
	state, err := s.extensions.InstallMounted(r.Context(), in.SHA256, in.Grants)
	if err != nil {
		extensionReply(w, err)
		return
	}
	writeJSON(w, 200, state)
}
func (s *Server) installExtension(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, extension.MaxPackageBytes+1<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid extension upload"})
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, _, err := r.FormFile("package")
	if err != nil {
		extensionReply(w, err)
		return
	}
	defer f.Close()
	tmp, err := os.CreateTemp("", "cph-extension-*.cphext")
	if err != nil {
		extensionReply(w, err)
		return
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, io.LimitReader(f, extension.MaxPackageBytes+1))
	closeErr := tmp.Close()
	if err = errors.Join(err, closeErr); err != nil {
		extensionReply(w, err)
		return
	}
	if r.URL.Path == "/admin/extensions/inspect" {
		v, err := s.extensions.Inspect(tmp.Name())
		if err == nil {
			err = s.extensions.CheckManagement(r.Context(), v.Manifest)
		}
		if err != nil {
			extensionReply(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"manifest": v.Manifest, "hash": v.SHA256, "signer": v.Signature.KeyID, "publisher": v.Publisher})
		return
	}
	var grants []string
	if err := json.Unmarshal([]byte(r.FormValue("grants")), &grants); err != nil {
		writeJSON(w, 400, map[string]string{"error": "explicit permission grants required"})
		return
	}
	ticket, err := s.extensions.StageUpload(tmp.Name())
	if err != nil {
		extensionReply(w, err)
		return
	}
	defer s.extensions.DiscardUpload(ticket)
	input, _ := json.Marshal(map[string]any{"ticket": ticket, "grants": grants})
	state, err := s.actions.Invoke(r.Context(), s.principal(r), "core.extensions.install", input)
	if err != nil {
		extensionReply(w, err)
		return
	}
	writeJSON(w, 200, state)
}
func extensionReply(w http.ResponseWriter, err error) {
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) enableExtension(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !readBody(w, r, &body) {
		return
	}
	extensionReply(w, s.extensions.SetEnabled(r.Context(), r.PathValue("id"), body.Enabled))
}

func (s *Server) getExtensionSettings(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	settings, err := s.extensions.Settings(r.Context(), r.PathValue("id"))
	if err != nil {
		extensionReply(w, err)
		return
	}
	writeJSON(w, 200, settings)
}

func (s *Server) putExtensionSettings(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var body struct {
		Hash   string         `json:"hash"`
		Values map[string]any `json:"values"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, extension.MaxSettingsBytes+1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		extensionReply(w, err)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		extensionReply(w, errors.New("expected one extension settings object"))
		return
	}
	input, _ := json.Marshal(map[string]any{"id": r.PathValue("id"), "hash": body.Hash, "values": body.Values})
	settings, err := s.actions.Invoke(r.Context(), s.principal(r), "core.extensions.configure", input)
	if err != nil {
		extensionReply(w, err)
		return
	}
	writeJSON(w, 200, settings)
}
func (s *Server) uninstallExtension(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	extensionReply(w, s.extensions.Uninstall(r.Context(), r.PathValue("id")))
}
func (s *Server) cleanExtensionCache(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	extensionReply(w, s.extensions.CleanCache(r.Context(), r.PathValue("id")))
}
func (s *Server) cleanExtensionData(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	extensionReply(w, s.extensions.CleanData(r.Context(), r.PathValue("id")))
}
