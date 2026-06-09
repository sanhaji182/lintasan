package oauthide

import (
	"encoding/json"
	"fmt"
	"strings"
)

type CursorImportPayload struct {
	AccessToken string `json:"accessToken"`
	MachineID   string `json:"machineId"`
}

func CursorImportInstructions() map[string]any {
	return map[string]any{
		"provider": "cursor",
		"method":   "import_token",
		"paths": map[string]string{
			"linux":   "~/.config/Cursor/User/globalStorage/state.vscdb",
			"macos":   "~/Library/Application Support/Cursor/User/globalStorage/state.vscdb",
			"windows": "%APPDATA%\\Cursor\\User\\globalStorage/state.vscdb",
		},
		"keys": map[string]string{
			"accessToken": "cursorAuth/accessToken",
			"machineId":   "storage.serviceMachineId",
		},
	}
}

func ValidateCursorImport(accessToken, machineID string) (*CursorImportPayload, string, error) {
	accessToken = strings.TrimSpace(accessToken)
	machineID = strings.TrimSpace(machineID)
	if accessToken == "" {
		return nil, "", fmt.Errorf("accessToken is required")
	}
	if machineID == "" {
		return nil, "", fmt.Errorf("machineId is required")
	}
	meta, _ := json.Marshal(map[string]any{"machineId": machineID, "authMethod": "imported"})
	return &CursorImportPayload{AccessToken: accessToken, MachineID: machineID}, string(meta), nil
}