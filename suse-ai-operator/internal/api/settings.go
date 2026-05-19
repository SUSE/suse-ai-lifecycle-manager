/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	aiplatformv1alpha1 "github.com/SUSE/suse-ai-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const settingsName = "settings"
const settingsFieldOwner = "suse-ai-operator-api"

// SettingsHandler serves GET /api/v1/settings and PUT /api/v1/settings.
type SettingsHandler struct {
	client    client.Client
	namespace string
}

// NewSettingsHandler constructs a SettingsHandler.
func NewSettingsHandler(c client.Client, namespace string) *SettingsHandler {
	return &SettingsHandler{client: c, namespace: namespace}
}

// Register wires the handler's routes onto the mux.
func (h *SettingsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/settings", h.getSettings)
	mux.HandleFunc("PUT /api/v1/settings", h.putSettings)
}

func (h *SettingsHandler) getSettings(w http.ResponseWriter, r *http.Request) {
	var s aiplatformv1alpha1.Settings
	key := types.NamespacedName{Namespace: h.namespace, Name: settingsName}
	if err := h.client.Get(r.Context(), key, &s); err != nil {
		if errors.IsNotFound(err) {
			writeError(w, http.StatusNotFound, fmt.Errorf("%w: settings CR not found", ErrNotFound))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.ManagedFields = nil
	writeJSON(w, http.StatusOK, &s)
}

type settingsPutBody struct {
	Spec aiplatformv1alpha1.SettingsSpec `json:"spec"`
}

func (h *SettingsHandler) putSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, fmt.Errorf("%w: Content-Type must be application/json", ErrInvalidInput))
		return
	}

	var body settingsPutBody
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("%w: %v", ErrInvalidInput, err))
		return
	}

	s := &aiplatformv1alpha1.Settings{}
	s.APIVersion = "ai-platform.suse.com/v1alpha1"
	s.Kind = "Settings"
	s.Name = settingsName
	s.Namespace = h.namespace
	s.Spec = body.Spec

	if err := h.client.Patch(
		r.Context(), s, client.Apply,
		client.ForceOwnership,
		client.FieldOwner(settingsFieldOwner),
	); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.ManagedFields = nil
	writeJSON(w, http.StatusOK, s)
}

// Compile-time guard: SettingsHandler satisfies Handler.
var _ Handler = (*SettingsHandler)(nil)
