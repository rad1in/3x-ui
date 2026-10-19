package discord

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

type mockServerProvider struct {
	status   *service.Status
	dbData   []byte
	dbErr    error
	filename string
}

func (m *mockServerProvider) GetStatus(lastStatus *service.Status) *service.Status {
	return m.status
}

func (m *mockServerProvider) GetDb() ([]byte, error) {
	return m.dbData, m.dbErr
}

func (m *mockServerProvider) BackupFilename(requestHost string) string {
	if m.filename != "" {
		return m.filename
	}
	return "eternal_test.db"
}

type mockInboundProvider struct {
	inbounds []*model.Inbound
	err      error
}

func (m *mockInboundProvider) GetAllInbounds() ([]*model.Inbound, error) {
	return m.inbounds, m.err
}

func TestBuildReport_NoBackup(t *testing.T) {
	settingService := setupTestDB(t)
	_ = settingService.SetDiscordBotToken("test-bot-token")
	_ = settingService.SetDiscordChannelId("12345")
	_ = settingService.SetDiscordBotBackup(false)
	_ = settingService.SetDiscordRunTime("@daily")

	mockStatus := &service.Status{
		Uptime:   172800,
		Loads:    []float64{0.5, 0.4, 0.3},
		TcpCount: 15,
		UdpCount: 5,
	}
	mockStatus.Xray.State = service.Running
	mockStatus.Xray.Version = "25.1.0"

	mockServer := &mockServerProvider{
		status: mockStatus,
		dbData: []byte("sqlite-backup-bytes"),
	}

	mockInbound := &mockInboundProvider{
		inbounds: []*model.Inbound{
			{
				Id:     1,
				Remark: "VLESS-TCP",
				Enable: true,
				Port:   443,
				ClientStats: []xray.ClientTraffic{
					{Email: "user1@example.com", Enable: true, Up: 100, Down: 200},
					{Email: "user2@example.com", Enable: false},
				},
			},
		},
	}

	svc := NewDiscordService(settingService)
	payload, files, err := svc.BuildReport(context.Background(), mockServer, mockInbound)
	if err != nil {
		t.Fatalf("BuildReport failed: %v", err)
	}

	if len(payload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(payload.Embeds))
	}
	embed := payload.Embeds[0]
	if embed.Color != ColorBlue {
		t.Errorf("expected ColorBlue, got %X", embed.Color)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files when backup is disabled, got %d", len(files))
	}

	foundHost, foundUptime, foundXray := false, false, false
	for _, field := range embed.Fields {
		if field.Name == "Host" {
			foundHost = true
		}
		if field.Name == "Uptime" && field.Value == "2d 0h" {
			foundUptime = true
		}
		if field.Name == "Xray Core" && field.Value == "25.1.0 (running)" {
			foundXray = true
		}
	}
	if !foundHost || !foundUptime || !foundXray {
		t.Errorf("expected fields not found in embed: %+v", embed.Fields)
	}
}

func TestBuildReport_WithBackup(t *testing.T) {
	settingService := setupTestDB(t)
	_ = settingService.SetDiscordBotToken("test-bot-token")
	_ = settingService.SetDiscordChannelId("12345")
	_ = settingService.SetDiscordBotBackup(true)

	mockStatus := &service.Status{
		Uptime: 3600,
	}
	mockStatus.Xray.State = service.Running
	mockStatus.Xray.Version = "25.1.0"

	mockServer := &mockServerProvider{
		status:   mockStatus,
		dbData:   []byte("test-db-content"),
		filename: "eternal_backup.db",
	}

	svc := NewDiscordService(settingService)
	_, files, err := svc.BuildReport(context.Background(), mockServer, nil)
	if err != nil {
		t.Fatalf("BuildReport failed: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("expected at least 1 backup file, got 0")
	}
	if files[0].Filename != "eternal_backup.db" {
		t.Errorf("expected filename 'eternal_backup.db', got %q", files[0].Filename)
	}
	if string(files[0].Data) != "test-db-content" {
		t.Errorf("expected db content 'test-db-content', got %q", string(files[0].Data))
	}
}

func TestSendReport_Integration(t *testing.T) {
	settingService := setupTestDB(t)
	_ = settingService.SetDiscordBotToken("test-token")
	_ = settingService.SetDiscordChannelId("998877")
	_ = settingService.SetDiscordBotBackup(true)

	var receivedRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequest = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": "msg-123"}`))
	}))
	defer server.Close()

	svc := NewDiscordService(settingService)
	svc.SetBaseURL(server.URL)
	svc.SetHTTPClient(server.Client())

	mockStatus := &service.Status{
		Uptime: 86400,
	}
	mockStatus.Xray.State = service.Running
	mockStatus.Xray.Version = "25.1.0"

	mockServer := &mockServerProvider{
		status: mockStatus,
		dbData: []byte("sqlite-data"),
	}

	err := svc.SendReport(context.Background(), mockServer, nil)
	if err != nil {
		t.Fatalf("SendReport failed: %v", err)
	}
	if !receivedRequest {
		t.Error("expected server to receive report request")
	}
}

func TestSendReport_DeliversEmbedWhenBackupUploadIsRejected(t *testing.T) {
	settingService := setupTestDB(t)
	_ = settingService.SetDiscordBotToken("test-token")
	_ = settingService.SetDiscordChannelId("998877")
	_ = settingService.SetDiscordBotBackup(true)

	embeds := make(chan int, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write([]byte(`{"message": "Request entity too large", "code": 40005}`))
			return
		}
		var p MessagePayload
		_ = json.NewDecoder(r.Body).Decode(&p)
		embeds <- len(p.Embeds)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc := NewDiscordService(settingService)
	svc.SetBaseURL(server.URL)
	svc.SetHTTPClient(server.Client())

	mockServer := &mockServerProvider{
		status: &service.Status{Uptime: 86400},
		dbData: []byte("sqlite-data-over-the-upload-cap"),
	}

	err := svc.SendReport(context.Background(), mockServer, nil)
	if err == nil || !strings.Contains(err.Error(), "(413)") {
		t.Fatalf("SendReport error = %v, want the rejected backup upload (413)", err)
	}
	select {
	case n := <-embeds:
		if n != 1 {
			t.Fatalf("report message carried %d embeds, want 1", n)
		}
	default:
		t.Fatal("report embed was never delivered: it rode on the rejected backup upload")
	}
}

// A backup over Discord's upload cap arrives as gzip parts under the cap, the
// first one telling how to rejoin them, instead of one rejected upload.
func TestSendReport_SplitsABackupOverTheUploadCap(t *testing.T) {
	settingService := setupTestDB(t)
	_ = settingService.SetDiscordBotToken("test-token")
	_ = settingService.SetDiscordChannelId("998877")
	_ = settingService.SetDiscordBotBackup(true)
	orig := discordUploadLimit
	discordUploadLimit = 2048
	t.Cleanup(func() { discordUploadLimit = orig })

	type upload struct {
		content, name string
		size          int
	}
	var uploads []upload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse upload: %v", err)
			}
			var p MessagePayload
			_ = json.Unmarshal([]byte(r.FormValue("payload_json")), &p)
			for _, fhs := range r.MultipartForm.File {
				for _, fh := range fhs {
					uploads = append(uploads, upload{p.Content, fh.Filename, int(fh.Size)})
				}
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	svc := NewDiscordService(settingService)
	svc.SetBaseURL(server.URL)
	svc.SetHTTPClient(server.Client())

	db := make([]byte, 5000)
	_, _ = rand.Read(db)
	if err := svc.SendReport(context.Background(), &mockServerProvider{status: &service.Status{}, dbData: db, filename: "x.db"}, nil); err != nil {
		t.Fatal(err)
	}
	var backup []upload
	for _, u := range uploads {
		if strings.HasPrefix(u.name, "x.db") {
			backup = append(backup, u)
		}
	}
	if len(backup) != 3 || backup[0].name != "x.db.gz.part01" || backup[2].name != "x.db.gz.part03" {
		t.Fatalf("backup uploads = %+v; want three ordered parts", backup)
	}
	for _, u := range backup {
		if u.size > 2048 {
			t.Fatalf("%s is %d bytes, over the cap", u.name, u.size)
		}
	}
	if !strings.Contains(backup[0].content, "cat x.db.gz.part* | gunzip > x.db") {
		t.Fatalf("first part says %q; want the rejoin command", backup[0].content)
	}
}
