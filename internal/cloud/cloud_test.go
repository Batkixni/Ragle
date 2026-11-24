package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestCloudAuthMultiTenantSyncAndBlobIsolation(t *testing.T) {
	dataDir := t.TempDir()
	blobDir := t.TempDir()
	srv, err := Open(Options{
		DataDir:       dataDir,
		BlobDir:       blobDir,
		PublicHost:    "cloud.raglex.sorai.tw",
		QuotaBytes:    10 << 20,
		AllowRegister: true,
	})
	if err != nil {
		t.Fatalf("Open cloud server failed: %v", err)
	}
	defer srv.Close()
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// 1. Health check.
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz failed: err=%v status=%d", err, resp.StatusCode)
	}
	_ = resp.Body.Close()

	// 2. Register User A and User B.
	register := func(email, pw string) string {
		body, _ := json.Marshal(map[string]string{"email": email, "password": pw})
		r, err := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		if r.StatusCode != http.StatusCreated {
			t.Fatalf("register %s status=%d", email, r.StatusCode)
		}
		var out struct {
			OK    bool   `json:"ok"`
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&out)
		if !out.OK || out.Token == "" {
			t.Fatalf("register %s returned empty token", email)
		}
		return out.Token
	}

	tokA := register("alice@sorai.tw", "secret123")
	tokB := register("bob@sorai.tw", "secret456")

	// 3. Login works & rejects wrong password.
	loginBody, _ := json.Marshal(map[string]string{"email": "alice@sorai.tw", "password": "wrong"})
	rBad, _ := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if rBad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 on bad password, got %d", rBad.StatusCode)
	}
	_ = rBad.Body.Close()

	loginOK, _ := json.Marshal(map[string]string{"email": "alice@sorai.tw", "password": "secret123"})
	rGood, _ := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(loginOK))
	if rGood.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on valid login, got %d", rGood.StatusCode)
	}
	_ = rGood.Body.Close()

	// 4. User A uploads a blob; User A can read it, User B MUST get 404 (tenant isolation).
	payload := []byte("raglex-cloud-secret-artwork-bytes")
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])

	putReq, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/blobs/"+hash, bytes.NewReader(payload))
	putReq.Header.Set("Authorization", "Bearer "+tokA)
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil || putResp.StatusCode != http.StatusOK {
		t.Fatalf("User A blob upload failed: err=%v status=%d", err, putResp.StatusCode)
	}
	_ = putResp.Body.Close()

	getAReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/blobs/"+hash, nil)
	getAReq.Header.Set("Authorization", "Bearer "+tokA)
	getAResp, _ := http.DefaultClient.Do(getAReq)
	if getAResp.StatusCode != http.StatusOK {
		t.Fatalf("User A blob download expected 200, got %d", getAResp.StatusCode)
	}
	_ = getAResp.Body.Close()

	getBReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/blobs/"+hash, nil)
	getBReq.Header.Set("Authorization", "Bearer "+tokB)
	getBResp, _ := http.DefaultClient.Do(getBReq)
	if getBResp.StatusCode != http.StatusNotFound {
		t.Fatalf("User B should NOT see User A's blob: expected 404, got %d", getBResp.StatusCode)
	}
	_ = getBResp.Body.Close()

	// 5. User A pushes sync ops; User B's sync oplog stays empty (isolated).
	pushBody := []byte(`{"ops":[{"op_id":"op-1","device_id":"mac","timestamp":1,"kind":"asset.upsert","entity_id":"a1","payload":{"id":"a1"}}]}`)
	pushReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/sync/push", bytes.NewReader(pushBody))
	pushReq.Header.Set("Authorization", "Bearer "+tokA)
	pushReq.Header.Set("Content-Type", "application/json")
	pushResp, _ := http.DefaultClient.Do(pushReq)
	if pushResp.StatusCode != http.StatusOK {
		t.Fatalf("User A sync push failed: %d", pushResp.StatusCode)
	}
	_ = pushResp.Body.Close()

	pullBReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/sync/pull?since=0", nil)
	pullBReq.Header.Set("Authorization", "Bearer "+tokB)
	pullBResp, _ := http.DefaultClient.Do(pullBReq)
	var pullB struct {
		Ops []any `json:"ops"`
	}
	_ = json.NewDecoder(pullBResp.Body).Decode(&pullB)
	_ = pullBResp.Body.Close()
	if len(pullB.Ops) != 0 {
		t.Fatalf("expected isolated User B oplog empty, got %d ops", len(pullB.Ops))
	}
}

func TestCloudInviteOnlyRegistrationAndAdminAPI(t *testing.T) {
	srv, err := Open(Options{
		DataDir:       t.TempDir(),
		BlobDir:       t.TempDir(),
		PublicHost:    "cloud.raglex.sorai.tw",
		AllowRegister: true,
		InviteOnly:    true,
		AdminToken:    "super-secret-admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// 1. Registration without invite code MUST be rejected with 403.
	noInvBody := []byte(`{"email":"stranger@example.com","password":"password123"}`)
	rNoInv, err := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(noInvBody))
	if err != nil || rNoInv.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 without invite code, got err=%v status=%d", err, rNoInv.StatusCode)
	}
	_ = rNoInv.Body.Close()

	// 2. Admin API rejects unauthorized callers.
	rUnauth, _ := http.Get(ts.URL + "/api/v1/admin/invites")
	if rUnauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauth admin call, got %d", rUnauth.StatusCode)
	}
	_ = rUnauth.Body.Close()

	// 3. Admin creates a 1-use invite code.
	mkReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/invites", bytes.NewReader([]byte(`{"max_uses":1,"note":"owner"}`)))
	mkReq.Header.Set("Authorization", "Bearer super-secret-admin")
	mkResp, err := http.DefaultClient.Do(mkReq)
	if err != nil || mkResp.StatusCode != http.StatusCreated {
		t.Fatalf("admin create invite failed: err=%v status=%d", err, mkResp.StatusCode)
	}
	var mkOut struct {
		OK     bool       `json:"ok"`
		Invite InviteCode `json:"invite"`
	}
	_ = json.NewDecoder(mkResp.Body).Decode(&mkOut)
	_ = mkResp.Body.Close()
	if !mkOut.OK || mkOut.Invite.Code == "" {
		t.Fatalf("expected generated invite code, got %+v", mkOut)
	}

	// 4. First user registers cleanly with the invite code (case-insensitive).
	reg1, _ := json.Marshal(map[string]string{
		"email":       "owner@sorai.tw",
		"password":    "password123",
		"invite_code": mkOut.Invite.Code,
	})
	r1, _ := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(reg1))
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 with valid invite code, got %d", r1.StatusCode)
	}
	_ = r1.Body.Close()

	// 5. Reusing the exhausted 1-use code MUST fail with 403.
	reg2, _ := json.Marshal(map[string]string{
		"email":       "second@sorai.tw",
		"password":    "password123",
		"invite_code": mkOut.Invite.Code,
	})
	r2, _ := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(reg2))
	if r2.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 on exhausted invite code, got %d", r2.StatusCode)
	}
	_ = r2.Body.Close()
}

type mockMail struct {
	to, subject, text, html string
}

type mockMailer struct {
	sent []mockMail
}

func (m *mockMailer) Enabled() bool { return true }
func (m *mockMailer) Send(_ context.Context, to, subject, text, htmlBody string) error {
	m.sent = append(m.sent, mockMail{to: to, subject: subject, text: text, html: htmlBody})
	return nil
}

func TestCloudEmailVerificationPasswordResetAndInviteMail(t *testing.T) {
	mailer := &mockMailer{}
	srv, err := Open(Options{
		DataDir:       t.TempDir(),
		BlobDir:       t.TempDir(),
		PublicHost:    "cloud.raglex.sorai.tw",
		AllowRegister: true,
		InviteOnly:    true,
		AdminToken:    "adm-secret",
		Mailer:        mailer,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// 1. Admin creates invite AND emails it to friend@sorai.tw.
	mkReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/invites", bytes.NewReader([]byte(`{"max_uses":1,"note":"friend","send_to":"friend@sorai.tw"}`)))
	mkReq.Header.Set("Authorization", "Bearer adm-secret")
	mkResp, _ := http.DefaultClient.Do(mkReq)
	var mkOut struct {
		OK      bool       `json:"ok"`
		Emailed bool       `json:"emailed"`
		Invite  InviteCode `json:"invite"`
	}
	_ = json.NewDecoder(mkResp.Body).Decode(&mkOut)
	_ = mkResp.Body.Close()
	if !mkOut.OK || !mkOut.Emailed || len(mailer.sent) != 1 || !strings.Contains(mailer.sent[0].text, mkOut.Invite.Code) {
		t.Fatalf("invite email failed: out=%+v sent=%d", mkOut, len(mailer.sent))
	}

	// 2. Register triggers automatic verification email with 6-digit code.
	regBody, _ := json.Marshal(map[string]string{
		"email":       "friend@sorai.tw",
		"password":    "oldpass123",
		"invite_code": mkOut.Invite.Code,
	})
	regResp, _ := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(regBody))
	var regOut struct {
		Token            string `json:"token"`
		VerificationSent bool   `json:"verification_sent"`
	}
	_ = json.NewDecoder(regResp.Body).Decode(&regOut)
	_ = regResp.Body.Close()
	if !regOut.VerificationSent || len(mailer.sent) != 2 {
		t.Fatalf("expected verification email sent on register, sent=%d", len(mailer.sent))
	}
	sixDigit := regexp.MustCompile(`\b(\d{6})\b`)
	verifyMatch := sixDigit.FindStringSubmatch(mailer.sent[1].text)
	if len(verifyMatch) < 2 {
		t.Fatalf("no 6-digit verification code in email: %s", mailer.sent[1].text)
	}

	// 3. Verify email using the 6-digit code.
	vBody, _ := json.Marshal(map[string]string{"code": verifyMatch[1]})
	vReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/verify-email", bytes.NewReader(vBody))
	vReq.Header.Set("Authorization", "Bearer "+regOut.Token)
	vResp, _ := http.DefaultClient.Do(vReq)
	if vResp.StatusCode != http.StatusOK {
		t.Fatalf("verify-email failed: %d", vResp.StatusCode)
	}
	_ = vResp.Body.Close()

	// 4. Forgot password sends 6-digit reset code.
	fpResp, _ := http.Post(ts.URL+"/api/v1/auth/forgot-password", "application/json", bytes.NewReader([]byte(`{"email":"friend@sorai.tw"}`)))
	if fpResp.StatusCode != http.StatusOK || len(mailer.sent) != 3 {
		t.Fatalf("forgot-password failed: status=%d sent=%d", fpResp.StatusCode, len(mailer.sent))
	}
	_ = fpResp.Body.Close()
	resetMatch := sixDigit.FindStringSubmatch(mailer.sent[2].text)
	if len(resetMatch) < 2 {
		t.Fatalf("no 6-digit reset code in email: %s", mailer.sent[2].text)
	}

	// 5. Reset password succeeds, revokes old token, and allows login with new password.
	rpBody, _ := json.Marshal(map[string]string{
		"email":        "friend@sorai.tw",
		"code":         resetMatch[1],
		"new_password": "newpass456",
	})
	rpResp, _ := http.Post(ts.URL+"/api/v1/auth/reset-password", "application/json", bytes.NewReader(rpBody))
	if rpResp.StatusCode != http.StatusOK {
		t.Fatalf("reset-password failed: %d", rpResp.StatusCode)
	}
	_ = rpResp.Body.Close()

	// Old token must now be revoked (401).
	meOld, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/auth/me", nil)
	meOld.Header.Set("Authorization", "Bearer "+regOut.Token)
	meOldResp, _ := http.DefaultClient.Do(meOld)
	if meOldResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected old token revoked after password reset (401), got %d", meOldResp.StatusCode)
	}
	_ = meOldResp.Body.Close()

	// Login with new password succeeds.
	loginNew, _ := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader([]byte(`{"email":"friend@sorai.tw","password":"newpass456"}`)))
	if loginNew.StatusCode != http.StatusOK {
		t.Fatalf("login with new password failed: %d", loginNew.StatusCode)
	}
	_ = loginNew.Body.Close()
}
