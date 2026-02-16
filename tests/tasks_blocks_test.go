package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"berkut-scc/config"
	"berkut-scc/core/auth"
	"berkut-scc/core/rbac"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
	"berkut-scc/tasks"
	taskhttp "berkut-scc/tasks/http"
	taskstore "berkut-scc/tasks/store"
)

type taskEnv struct {
	ctx        context.Context
	cfg        *config.AppConfig
	users      store.UsersStore
	tasksStore *taskstore.SQLStore
	handler    *taskhttp.Handler
	admin      *store.User
	analyst    *store.User
	board      *tasks.Board
	todo       *tasks.Column
	done       *tasks.Column
	cleanup    func()
}

func setupTasksEnv(t *testing.T) *taskEnv {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.AppConfig{DBPath: filepath.Join(dir, "test.db")}
	logger := utils.NewLogger()
	db, err := store.NewDB(cfg, logger)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.ApplyMigrations(context.Background(), db, logger); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := store.NewUsersStore(db)
	audits := store.NewAuditStore(db)
	tasksStore := taskstore.NewStore(db)
	policy := rbac.NewPolicy(rbac.DefaultRoles())
	handler := taskhttp.NewHandler(cfg, tasks.NewService(tasksStore), users, nil, nil, nil, nil, nil, nil, policy, audits)

	admin := &store.User{
		Username:       "admin1",
		FullName:       "Admin User",
		Department:     "Sec",
		Position:       "Admin",
		ClearanceLevel: 1,
		PasswordHash:   "hash",
		Salt:           "salt",
		PasswordSet:    true,
		Active:         true,
	}
	adminID, err := users.Create(context.Background(), admin, []string{"admin"})
	if err != nil {
		t.Fatalf("admin create: %v", err)
	}
	admin.ID = adminID

	analyst := &store.User{
		Username:       "analyst1",
		FullName:       "Analyst User",
		Department:     "Sec",
		Position:       "Analyst",
		ClearanceLevel: 1,
		PasswordHash:   "hash",
		Salt:           "salt",
		PasswordSet:    true,
		Active:         true,
	}
	analystID, err := users.Create(context.Background(), analyst, []string{"analyst"})
	if err != nil {
		t.Fatalf("analyst create: %v", err)
	}
	analyst.ID = analystID

	board := &tasks.Board{
		SpaceID:   1,
		Name:      "Board",
		IsActive:  true,
		CreatedBy: &admin.ID,
	}
	space := &tasks.Space{
		Name:      "Default space",
		IsActive:  true,
		CreatedBy: &admin.ID,
	}
	spaceID, err := tasksStore.CreateSpace(context.Background(), space, nil)
	if err != nil {
		t.Fatalf("space create: %v", err)
	}
	board.SpaceID = spaceID
	acl := []tasks.ACLRule{
		{SubjectType: "user", SubjectID: admin.Username, Permission: "manage"},
		{SubjectType: "user", SubjectID: analyst.Username, Permission: "manage"},
	}
	if _, err := tasksStore.CreateBoard(context.Background(), board, acl); err != nil {
		t.Fatalf("board create: %v", err)
	}
	todo := &tasks.Column{
		BoardID:  board.ID,
		Name:     "Todo",
		Position: 1,
		IsActive: true,
	}
	if _, err := tasksStore.CreateColumn(context.Background(), todo); err != nil {
		t.Fatalf("todo create: %v", err)
	}
	done := &tasks.Column{
		BoardID:  board.ID,
		Name:     "Done",
		Position: 2,
		IsFinal:  true,
		IsActive: true,
	}
	if _, err := tasksStore.CreateColumn(context.Background(), done); err != nil {
		t.Fatalf("done create: %v", err)
	}

	return &taskEnv{
		ctx:        context.Background(),
		cfg:        cfg,
		users:      users,
		tasksStore: tasksStore,
		handler:    handler,
		admin:      admin,
		analyst:    analyst,
		board:      board,
		todo:       todo,
		done:       done,
		cleanup: func() {
			db.Close()
		},
	}
}

func createTask(t *testing.T, env *taskEnv, title string) *tasks.Task {
	t.Helper()
	task := &tasks.Task{
		BoardID:    env.board.ID,
		ColumnID:   env.todo.ID,
		Title:      title,
		Priority:   tasks.PriorityMedium,
		CreatedBy:  &env.admin.ID,
		IsArchived: false,
	}
	if _, err := env.tasksStore.CreateTask(env.ctx, task, nil); err != nil {
		t.Fatalf("task create: %v", err)
	}
	return task
}

func authedRequest(method, path string, body []byte, user *store.User) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), auth.SessionContextKey, &store.SessionRecord{
		UserID:   user.ID,
		Username: user.Username,
	}))
	return req
}

func TestTextBlockBlocksFinalAndClose(t *testing.T) {
	env := setupTasksEnv(t)
	defer env.cleanup()
	task := createTask(t, env, "Blocked task")

	payload, _ := json.Marshal(map[string]string{"reason": "Need info"})
	req := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(task.ID, 10)+"/blocks/text", payload, env.admin)
	req = withURLParams(req, map[string]string{"id": strconv.FormatInt(task.ID, 10)})
	rr := httptest.NewRecorder()
	env.handler.AddTextBlock(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected create block 201, got %d", rr.Code)
	}

	movePayload, _ := json.Marshal(map[string]any{"column_id": env.done.ID, "position": 1})
	moveReq := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(task.ID, 10)+"/move", movePayload, env.admin)
	moveReq = withURLParams(moveReq, map[string]string{"id": strconv.FormatInt(task.ID, 10)})
	moveRR := httptest.NewRecorder()
	env.handler.MoveTask(moveRR, moveReq)
	if moveRR.Code != http.StatusConflict {
		t.Fatalf("expected move blocked 409, got %d", moveRR.Code)
	}

	closeReq := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(task.ID, 10)+"/close", nil, env.admin)
	closeReq = withURLParams(closeReq, map[string]string{"id": strconv.FormatInt(task.ID, 10)})
	closeRR := httptest.NewRecorder()
	env.handler.CloseTask(closeRR, closeReq)
	if closeRR.Code != http.StatusConflict {
		t.Fatalf("expected close blocked 409, got %d", closeRR.Code)
	}
}

func TestTaskBlockBlocksFinal(t *testing.T) {
	env := setupTasksEnv(t)
	defer env.cleanup()
	blocker := createTask(t, env, "Blocker")
	target := createTask(t, env, "Target")

	payload, _ := json.Marshal(map[string]any{"blocker_task_id": blocker.ID})
	req := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(target.ID, 10)+"/blocks/task", payload, env.admin)
	req = withURLParams(req, map[string]string{"id": strconv.FormatInt(target.ID, 10)})
	rr := httptest.NewRecorder()
	env.handler.AddTaskBlock(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected create task block 201, got %d", rr.Code)
	}

	movePayload, _ := json.Marshal(map[string]any{"column_id": env.done.ID, "position": 1})
	moveReq := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(target.ID, 10)+"/move", movePayload, env.admin)
	moveReq = withURLParams(moveReq, map[string]string{"id": strconv.FormatInt(target.ID, 10)})
	moveRR := httptest.NewRecorder()
	env.handler.MoveTask(moveRR, moveReq)
	if moveRR.Code != http.StatusConflict {
		t.Fatalf("expected move blocked 409, got %d", moveRR.Code)
	}
}

func TestAutoResolveOnClose(t *testing.T) {
	env := setupTasksEnv(t)
	defer env.cleanup()
	blocker := createTask(t, env, "Blocker")
	target := createTask(t, env, "Target")

	payload, _ := json.Marshal(map[string]any{"blocker_task_id": blocker.ID})
	req := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(target.ID, 10)+"/blocks/task", payload, env.admin)
	req = withURLParams(req, map[string]string{"id": strconv.FormatInt(target.ID, 10)})
	rr := httptest.NewRecorder()
	env.handler.AddTaskBlock(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected create task block 201, got %d", rr.Code)
	}

	closeReq := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(blocker.ID, 10)+"/close", nil, env.admin)
	closeReq = withURLParams(closeReq, map[string]string{"id": strconv.FormatInt(blocker.ID, 10)})
	closeRR := httptest.NewRecorder()
	env.handler.CloseTask(closeRR, closeReq)
	if closeRR.Code != http.StatusOK {
		t.Fatalf("expected close 200, got %d", closeRR.Code)
	}

	blocks, err := env.tasksStore.ListTaskBlocks(env.ctx, target.ID)
	if err != nil || len(blocks) == 0 {
		t.Fatalf("blocks missing: %v", err)
	}
	if blocks[0].IsActive {
		t.Fatalf("expected block auto resolved")
	}
	if blocks[0].ResolvedBy == nil || *blocks[0].ResolvedBy != env.admin.ID {
		t.Fatalf("expected resolved_by to be set")
	}
}

func TestBlockCycleDetection(t *testing.T) {
	env := setupTasksEnv(t)
	defer env.cleanup()
	taskA := createTask(t, env, "A")
	taskB := createTask(t, env, "B")
	taskC := createTask(t, env, "C")

	payloadAB, _ := json.Marshal(map[string]any{"blocker_task_id": taskA.ID})
	reqAB := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(taskB.ID, 10)+"/blocks/task", payloadAB, env.admin)
	reqAB = withURLParams(reqAB, map[string]string{"id": strconv.FormatInt(taskB.ID, 10)})
	rrAB := httptest.NewRecorder()
	env.handler.AddTaskBlock(rrAB, reqAB)
	if rrAB.Code != http.StatusCreated {
		t.Fatalf("expected A->B block 201, got %d", rrAB.Code)
	}

	payloadBC, _ := json.Marshal(map[string]any{"blocker_task_id": taskB.ID})
	reqBC := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(taskC.ID, 10)+"/blocks/task", payloadBC, env.admin)
	reqBC = withURLParams(reqBC, map[string]string{"id": strconv.FormatInt(taskC.ID, 10)})
	rrBC := httptest.NewRecorder()
	env.handler.AddTaskBlock(rrBC, reqBC)
	if rrBC.Code != http.StatusCreated {
		t.Fatalf("expected B->C block 201, got %d", rrBC.Code)
	}

	payloadCA, _ := json.Marshal(map[string]any{"blocker_task_id": taskC.ID})
	reqCA := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(taskA.ID, 10)+"/blocks/task", payloadCA, env.admin)
	reqCA = withURLParams(reqCA, map[string]string{"id": strconv.FormatInt(taskA.ID, 10)})
	rrCA := httptest.NewRecorder()
	env.handler.AddTaskBlock(rrCA, reqCA)
	if rrCA.Code != http.StatusConflict {
		t.Fatalf("expected cycle conflict 409, got %d", rrCA.Code)
	}
}

func TestBlockRBAC(t *testing.T) {
	env := setupTasksEnv(t)
	defer env.cleanup()
	task := createTask(t, env, "RBAC task")

	payload, _ := json.Marshal(map[string]string{"reason": "Need info"})
	req := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(task.ID, 10)+"/blocks/text", payload, env.analyst)
	req = withURLParams(req, map[string]string{"id": strconv.FormatInt(task.ID, 10)})
	rr := httptest.NewRecorder()
	env.handler.AddTextBlock(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden create, got %d", rr.Code)
	}

	adminReq := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(task.ID, 10)+"/blocks/text", payload, env.admin)
	adminReq = withURLParams(adminReq, map[string]string{"id": strconv.FormatInt(task.ID, 10)})
	adminRR := httptest.NewRecorder()
	env.handler.AddTextBlock(adminRR, adminReq)
	if adminRR.Code != http.StatusCreated {
		t.Fatalf("expected create block 201, got %d", adminRR.Code)
	}
	var created tasks.TaskBlock
	if err := json.Unmarshal(adminRR.Body.Bytes(), &created); err != nil || created.ID == 0 {
		t.Fatalf("expected block payload")
	}

	resolveReq := authedRequest("POST", "/api/tasks/"+strconv.FormatInt(task.ID, 10)+"/blocks/"+strconv.FormatInt(created.ID, 10)+"/resolve", nil, env.analyst)
	resolveReq = withURLParams(resolveReq, map[string]string{
		"id":       strconv.FormatInt(task.ID, 10),
		"block_id": strconv.FormatInt(created.ID, 10),
	})
	resolveRR := httptest.NewRecorder()
	env.handler.ResolveBlock(resolveRR, resolveReq)
	if resolveRR.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden resolve, got %d", resolveRR.Code)
	}
}
