//go:build integration

package integration

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFunctionsZipDeployment(t *testing.T) {
	baseURL, projectID := strings.TrimRight(os.Getenv("SUPABASE_MANAGER_E2E_URL"), "/"), os.Getenv("SUPABASE_MANAGER_E2E_PROJECT_ID")
	cookie, csrf := os.Getenv("SUPABASE_MANAGER_E2E_COOKIE"), os.Getenv("SUPABASE_MANAGER_E2E_CSRF")
	if baseURL == "" || projectID == "" || cookie == "" || csrf == "" {
		t.Skip("set SUPABASE_MANAGER_E2E_URL, PROJECT_ID, COOKIE, and CSRF for function acceptance")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	first := functionZIP(t, "one")
	deploy := multipartRequest(t, baseURL+"/api/projects/"+projectID+"/functions/demo/deploy", first, cookie, csrf)
	waitOperation(t, client, baseURL, deploy, cookie, csrf)
	second := functionZIP(t, "two")
	deploy = multipartRequest(t, baseURL+"/api/projects/"+projectID+"/functions/demo/deploy", second, cookie, csrf)
	waitOperation(t, client, baseURL, deploy, cookie, csrf)
	functions := getJSON(t, client, baseURL+"/api/projects/"+projectID+"/functions", cookie, csrf)
	if len(functions["functions"].([]any)) != 1 {
		t.Fatalf("functions list = %#v", functions)
	}
	body, _ := json.Marshal(map[string]string{})
	request, _ := http.NewRequest(http.MethodPost, baseURL+"/api/projects/"+projectID+"/functions/demo/rollback", bytes.NewReader(body))
	request.Header.Set("Origin", baseURL)
	request.Header.Set("Cookie", cookie)
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("rollback status=%d", response.StatusCode)
	}
}

func functionZIP(t *testing.T, body string) []byte {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("index.ts")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("export const marker = '" + body + "'"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func multipartRequest(t *testing.T, rawURL string, archive []byte, cookie, csrf string) string {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("archive", "demo.zip")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(archive)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodPost, rawURL, &body)
	request.Header.Set("Origin", strings.SplitN(rawURL, "/api/", 2)[0])
	request.Header.Set("Cookie", cookie)
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("deploy status=%d", response.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result["operationId"].(string)
}
