package api

import (
	"net/http"
	"strings"

	"fastcopy/server/internal/hub"
)

type renameDeviceRequest struct {
	Name string `json:"name"`
}

func (a *API) devices(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r)
	devices, err := a.store.Devices(r.Context(), principal.UserID, principal.DeviceID)
	if err != nil {
		storeError(w, err)
		return
	}
	for index := range devices {
		devices[index].Online = a.hub.IsOnline(principal.UserID, devices[index].ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (a *API) renameDevice(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r)
	var request renameDeviceRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if len([]rune(request.Name)) > 64 {
		writeError(w, http.StatusBadRequest, "INVALID_DEVICE_NAME", "device name must contain at most 64 characters")
		return
	}
	device, err := a.store.RenameDevice(r.Context(), principal.UserID, r.PathValue("deviceID"), request.Name)
	if err != nil {
		storeError(w, err)
		return
	}
	device.Current = device.ID == principal.DeviceID
	device.Online = a.hub.IsOnline(principal.UserID, device.ID)
	a.hub.Publish(principal.UserID, "", hub.Event{Type: "device.updated", Data: device})
	writeJSON(w, http.StatusOK, device)
}

func (a *API) revokeDevice(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r)
	deviceID := r.PathValue("deviceID")
	if err := a.store.RevokeDevice(r.Context(), principal.UserID, deviceID); err != nil {
		storeError(w, err)
		return
	}
	a.hub.Publish(principal.UserID, deviceID, hub.Event{
		Type: "device.revoked",
		Data: map[string]string{"device_id": deviceID},
	})
	a.hub.CloseDevice(principal.UserID, deviceID)
	w.WriteHeader(http.StatusNoContent)
}
