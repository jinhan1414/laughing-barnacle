package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/skills"
)

func TestHandleAPIMaintenanceManage_Success(t *testing.T) {
	root := t.TempDir()
	mcpStore, err := mcp.NewStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("New skill store error: %v", err)
	}
	runtime := &mockScheduleRuntime{}
	s := &Server{mcpStore: mcpStore, skillStore: skillStore, scheduler: runtime}

	callAPI := func(handler func(http.ResponseWriter, *http.Request), path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec
	}

	if rec := callAPI(s.handleAPIMCPServiceSave, "/api/mcp/services/save", `{"name":"svc-a","transport":"streamable_http","endpoint":"http://127.0.0.1:7777/mcp","enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("mcp save expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	services := mcpStore.ListServices()
	if len(services) != 1 {
		t.Fatalf("expected one mcp service, got %d", len(services))
	}
	serviceID := services[0].ID
	if rec := callAPI(s.handleAPIMCPServiceToggle, "/api/mcp/services/toggle", `{"id":"`+serviceID+`","enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("mcp toggle expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	service, ok := mcpStore.GetService(serviceID)
	if !ok || service.Enabled {
		t.Fatalf("expected mcp service disabled after toggle")
	}
	if rec := callAPI(s.handleAPIMCPServiceDelete, "/api/mcp/services/delete", `{"id":"`+serviceID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("mcp delete expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(mcpStore.ListServices()) != 0 {
		t.Fatalf("expected no mcp services after delete")
	}

	if rec := callAPI(s.handleAPISkillSave, "/api/skills/save", `{"id":"daily-skill","name":"daily-skill","description":"daily","prompt":"do daily","enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("skill save expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := callAPI(s.handleAPISkillToggle, "/api/skills/toggle", `{"id":"daily-skill","enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("skill toggle expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := skillStore.ReadEnabledSkillPrompt("daily-skill"); !ok {
		t.Fatalf("expected daily-skill enabled")
	}

	saveScheduleBody := `{"id":"daily-task","name":"daily-task","description":"daily run","action":"skill:daily-skill","cron_expr":"0 9 * * *","enabled":true}`
	if rec := callAPI(s.handleAPIScheduleSave, "/api/schedules/save", saveScheduleBody); rec.Code != http.StatusOK {
		t.Fatalf("schedule save expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	tasks := mcpStore.ListScheduledTasks()
	if len(tasks) != 1 || tasks[0].ID != "daily-task" {
		t.Fatalf("expected one daily-task schedule, got %+v", tasks)
	}

	if rec := callAPI(s.handleAPIScheduleToggle, "/api/schedules/toggle", `{"id":"daily-task","enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("schedule toggle expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	toggledTask, found := findScheduledTaskByID(mcpStore.ListScheduledTasks(), "daily-task")
	if !found || toggledTask.Enabled {
		t.Fatalf("expected daily-task disabled after toggle")
	}

	if rec := callAPI(s.handleAPIScheduleRun, "/api/schedules/run", `{"id":"daily-task"}`); rec.Code != http.StatusOK {
		t.Fatalf("schedule run expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if runtime.ranTaskID != "daily-task" {
		t.Fatalf("expected scheduler runtime called with daily-task, got %q", runtime.ranTaskID)
	}

	if rec := callAPI(s.handleAPIScheduleDelete, "/api/schedules/delete", `{"id":"daily-task"}`); rec.Code != http.StatusOK {
		t.Fatalf("schedule delete expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if _, found := findScheduledTaskByID(mcpStore.ListScheduledTasks(), "daily-task"); found {
		t.Fatalf("expected daily-task deleted")
	}
	if _, ok := skillStore.ReadEnabledSkillPrompt("daily-skill"); ok {
		t.Fatalf("expected linked skill daily-skill deleted with schedule")
	}
}

func TestHandleAPIMaintenanceManage_ValidationErrors(t *testing.T) {
	root := t.TempDir()
	mcpStore, err := mcp.NewStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("New skill store error: %v", err)
	}
	s := &Server{mcpStore: mcpStore, skillStore: skillStore}

	testCases := []struct {
		name          string
		handler       func(http.ResponseWriter, *http.Request)
		path          string
		body          string
		errorContains string
	}{
		{
			name:          "invalid json body",
			handler:       s.handleAPIMCPServiceSave,
			path:          "/api/mcp/services/save",
			body:          `{"name":"svc-a"`,
			errorContains: "invalid json body",
		},
		{
			name:          "mcp toggle missing id",
			handler:       s.handleAPIMCPServiceToggle,
			path:          "/api/mcp/services/toggle",
			body:          `{"enabled":true}`,
			errorContains: "service id is required",
		},
		{
			name:          "skill save missing prompt",
			handler:       s.handleAPISkillSave,
			path:          "/api/skills/save",
			body:          `{"name":"demo","enabled":true}`,
			errorContains: "skill prompt is required",
		},
		{
			name:          "schedule save unknown skill",
			handler:       s.handleAPIScheduleSave,
			path:          "/api/schedules/save",
			body:          `{"id":"task-a","name":"task-a","description":"d","action":"skill:not-found","cron_expr":"*/5 * * * *","enabled":true}`,
			errorContains: "不存在或未启用",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			tc.handler(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			errMsg := decodeAPIErrorMessage(t, rec)
			if !strings.Contains(errMsg, tc.errorContains) {
				t.Fatalf("unexpected error: %q, want contains %q", errMsg, tc.errorContains)
			}
		})
	}
}

func TestMaintenanceSettingsAndAPIShareValidation(t *testing.T) {
	root := t.TempDir()
	mcpStore, err := mcp.NewStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("New skill store error: %v", err)
	}
	s := &Server{mcpStore: mcpStore, skillStore: skillStore}

	t.Run("mcp toggle validation same", func(t *testing.T) {
		form := url.Values{}
		form.Set("enabled", "true")
		settingsReq := httptest.NewRequest(http.MethodPost, "/settings/mcp/toggle", strings.NewReader(form.Encode()))
		settingsReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		settingsRec := httptest.NewRecorder()
		s.handleSettingsMCPToggle(settingsRec, settingsReq)
		settingsErr := decodeSettingsErrorMessage(t, settingsRec)

		apiReq := httptest.NewRequest(http.MethodPost, "/api/mcp/services/toggle", strings.NewReader(`{"enabled":true}`))
		apiReq.Header.Set("Content-Type", "application/json")
		apiRec := httptest.NewRecorder()
		s.handleAPIMCPServiceToggle(apiRec, apiReq)
		apiErr := decodeAPIErrorMessage(t, apiRec)

		if settingsErr != apiErr {
			t.Fatalf("expected same error, settings=%q api=%q", settingsErr, apiErr)
		}
	})

	t.Run("skill save validation same", func(t *testing.T) {
		form := url.Values{}
		form.Set("name", "demo")
		form.Set("enabled", "on")
		settingsReq := httptest.NewRequest(http.MethodPost, "/settings/skills/save", strings.NewReader(form.Encode()))
		settingsReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		settingsRec := httptest.NewRecorder()
		s.handleSettingsSkillSave(settingsRec, settingsReq)
		settingsErr := decodeSettingsErrorMessage(t, settingsRec)

		apiReq := httptest.NewRequest(http.MethodPost, "/api/skills/save", strings.NewReader(`{"name":"demo","enabled":true}`))
		apiReq.Header.Set("Content-Type", "application/json")
		apiRec := httptest.NewRecorder()
		s.handleAPISkillSave(apiRec, apiReq)
		apiErr := decodeAPIErrorMessage(t, apiRec)

		if settingsErr != apiErr {
			t.Fatalf("expected same error, settings=%q api=%q", settingsErr, apiErr)
		}
	})

	t.Run("schedule save validation same", func(t *testing.T) {
		form := url.Values{}
		form.Set("id", "task-a")
		form.Set("name", "task-a")
		form.Set("description", "d")
		form.Set("action", "skill:not-found")
		form.Set("cron_expr", "*/5 * * * *")
		form.Set("enabled", "on")
		settingsReq := httptest.NewRequest(http.MethodPost, "/settings/schedules/save", strings.NewReader(form.Encode()))
		settingsReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		settingsRec := httptest.NewRecorder()
		s.handleSettingsScheduleSave(settingsRec, settingsReq)
		settingsErr := decodeSettingsErrorMessage(t, settingsRec)

		apiBody := `{"id":"task-a","name":"task-a","description":"d","action":"skill:not-found","cron_expr":"*/5 * * * *","enabled":true}`
		apiReq := httptest.NewRequest(http.MethodPost, "/api/schedules/save", strings.NewReader(apiBody))
		apiReq.Header.Set("Content-Type", "application/json")
		apiRec := httptest.NewRecorder()
		s.handleAPIScheduleSave(apiRec, apiReq)
		apiErr := decodeAPIErrorMessage(t, apiRec)

		if settingsErr != apiErr {
			t.Fatalf("expected same error, settings=%q api=%q", settingsErr, apiErr)
		}
	})
}

func decodeAPIErrorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error: %v body=%s", err, rec.Body.String())
	}
	msg, _ := payload["error"].(string)
	if strings.TrimSpace(msg) == "" {
		t.Fatalf("expected non-empty error message, body=%s", rec.Body.String())
	}
	return msg
}

func decodeSettingsErrorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusFound {
		t.Fatalf("expected settings redirect 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	location := rec.Result().Header.Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect location error: %v location=%q", err, location)
	}
	msg := strings.TrimSpace(parsed.Query().Get("error"))
	if msg == "" {
		t.Fatalf("expected redirect error message, location=%q", location)
	}
	return msg
}
