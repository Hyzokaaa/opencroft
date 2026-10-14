package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/agent"
	"github.com/Hyzokaaa/opencroft/internal/autodeploy"
	"github.com/Hyzokaaa/opencroft/internal/shared/job"
)

// branchDeployer is a deployer whose service runs one commit while its
// branch ends at another.
type branchDeployer struct {
	*fakeDeployer
	service, deployed, tip string
}

func (b *branchDeployer) FindAll(context.Context, string) (agent.ServicesResponse, error) {
	return agent.ServicesResponse{Services: []agent.ServiceDTO{{Name: b.service, Commit: b.deployed}}}, nil
}

func (b *branchDeployer) Tip(context.Context, string, string) (string, error) { return b.tip, nil }

func autoDeployDeps(service, deployed, tip string) (Deps, *branchDeployer, *autodeploy.Memory) {
	fake := &branchDeployer{fakeDeployer: &fakeDeployer{}, service: service, deployed: deployed, tip: tip}
	repo := autodeploy.NewMemory()
	_ = repo.Save(context.Background(), autodeploy.Setting{Container: "helpdesk", Service: service, Secret: "s3cret"})
	next := 0
	jobs := job.NewRunner(func() string { next++; return fmt.Sprint(next) }, nil)
	return Deps{Services: fake, Jobs: jobs, AutoDeploys: NewAutoDeploys(repo)}, fake, repo
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("never happened: %s", what)
}

func idle(d Deps, service string) bool {
	d.AutoDeploys.mu.Lock()
	defer d.AutoDeploys.mu.Unlock()
	return !d.AutoDeploys.busy["helpdesk/"+service]
}

// The one route open without a session takes nothing but the secret.
func TestAWebhookWithoutTheSecretIsRefused(t *testing.T) {
	d, fake, _ := autoDeployDeps("backend", "aaa", "bbb")
	handler := api(d)

	for name, header := range map[string]http.Header{
		"no secret":        {},
		"wrong token":      {"X-Gitlab-Token": {"guess"}},
		"wrong signature":  {"X-Hub-Signature-256": {"sha256=00"}},
		"unsigned github":  {"X-Hub-Signature-256": {"s3cret"}},
		"secret elsewhere": {"Authorization": {"s3cret"}},
	} {
		request := httptest.NewRequest(http.MethodPost, "/hooks/deploy/helpdesk/backend", bytes.NewBufferString(`{}`))
		request.Header = header
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s: answered %d", name, recorder.Code)
		}
	}

	// A service that does not deploy itself has no secret to know.
	request := httptest.NewRequest(http.MethodPost, "/hooks/deploy/helpdesk/client", nil)
	request.Header.Set("X-Gitlab-Token", "s3cret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("a service that does not deploy itself answered %d", recorder.Code)
	}

	time.Sleep(50 * time.Millisecond)
	if fake.was("run redeploy helpdesk/backend") {
		t.Error("a refused webhook deployed")
	}
}

func TestAGitLabPushDeploysTheBranch(t *testing.T) {
	d, fake, _ := autoDeployDeps("backend", "aaa", "bbb")
	request := httptest.NewRequest(http.MethodPost, "/hooks/deploy/helpdesk/backend", bytes.NewBufferString(`{"ref":"refs/heads/dev"}`))
	request.Header.Set("X-Gitlab-Token", "s3cret")
	recorder := httptest.NewRecorder()
	api(d).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("answered %d: %s", recorder.Code, recorder.Body.String())
	}
	eventually(t, "the redeploy", func() bool { return fake.was("run redeploy helpdesk/backend") })
}

func TestAGitHubPushSignedWithTheSecretIsAccepted(t *testing.T) {
	d, fake, _ := autoDeployDeps("backend", "aaa", "bbb")
	body := []byte(`{"ref":"refs/heads/dev"}`)
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write(body)

	request := httptest.NewRequest(http.MethodPost, "/hooks/deploy/helpdesk/backend", bytes.NewReader(body))
	request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	recorder := httptest.NewRecorder()
	api(d).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("answered %d", recorder.Code)
	}
	eventually(t, "the redeploy", func() bool { return fake.was("run redeploy helpdesk/backend") })
}

func TestABranchThatDidNotMoveDeploysNothing(t *testing.T) {
	d, fake, _ := autoDeployDeps("backend", "abc1234", "abc1234def")
	d.checkBranch(context.Background(), "helpdesk", "backend", "check")

	if fake.was("plan redeploy helpdesk/backend") {
		t.Error("a service already at the end of its branch was deployed again")
	}
	if last := d.AutoDeploys.last["helpdesk/backend"]; last.Result != "up to date" {
		t.Errorf("the check says %q", last.Result)
	}
}

// A commit that fails is not tried on every check: it would rebuild the
// same broken code every few minutes. The next commit is tried.
func TestACommitThatFailedIsNotTriedAgain(t *testing.T) {
	d, fake, repo := autoDeployDeps("broken", "aaa", "bbb")
	d.checkBranch(context.Background(), "helpdesk", "broken", "check")
	eventually(t, "the first deployment ends", func() bool { return idle(d, "broken") })

	if setting, _ := repo.Find(context.Background(), "helpdesk", "broken"); setting.Failed != "bbb" {
		t.Fatalf("the failed commit is %q", setting.Failed)
	}

	fake.fakeDeployer.asked = nil
	d.checkBranch(context.Background(), "helpdesk", "broken", "check")
	if fake.was("run redeploy helpdesk/broken") || fake.was("plan redeploy helpdesk/broken") {
		t.Error("the commit that failed was deployed again")
	}

	fake.tip = "ccc"
	d.checkBranch(context.Background(), "helpdesk", "broken", "check")
	eventually(t, "the next commit is tried", func() bool { return fake.was("run redeploy helpdesk/broken") })
}

// Automatic deployments are recorded like any other, under their own kind.
func TestAnAutomaticDeploymentIsAJobLikeAnyOther(t *testing.T) {
	d, _, _ := autoDeployDeps("backend", "aaa", "bbb")
	d.checkBranch(context.Background(), "helpdesk", "backend", "check")
	eventually(t, "the deployment ends", func() bool { return idle(d, "backend") })

	recent, _ := d.Jobs.Recent(5)
	if len(recent) != 1 || recent[0].Kind != "auto-deploy" || recent[0].Subject != "helpdesk/backend" {
		t.Errorf("the jobs are %+v", recent)
	}
}

func TestTurningItOnMakesASecretAndOffForgetsIt(t *testing.T) {
	d, _, repo := autoDeployDeps("backend", "aaa", "aaa")
	_ = repo.Remove(context.Background(), "helpdesk", "backend")
	handler := api(d)

	put := func(enabled bool) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut,
			"/api/hosts/local/instances/helpdesk/services/backend/autodeploy",
			bytes.NewBufferString(fmt.Sprintf(`{"enabled":%t}`, enabled))))
		return recorder
	}

	if code := put(true).Code; code != http.StatusOK {
		t.Fatalf("turning it on answered %d", code)
	}
	first, _ := repo.Find(context.Background(), "helpdesk", "backend")
	if first == nil || len(first.Secret) < 32 {
		t.Fatalf("no secret worth the name: %+v", first)
	}

	put(false)
	if gone, _ := repo.Find(context.Background(), "helpdesk", "backend"); gone != nil {
		t.Fatal("turning it off kept the secret")
	}
	put(true)
	if again, _ := repo.Find(context.Background(), "helpdesk", "backend"); again.Secret == first.Secret {
		t.Error("turning it on again kept the old secret")
	}
}
