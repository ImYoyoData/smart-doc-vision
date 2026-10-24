package engine

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Calcium-Ion/moejs"
	men "github.com/Calcium-Ion/moejs/engine"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"smartdoc-vision/backend/internal/media"
	"smartdoc-vision/backend/internal/model"
)

type scriptRun struct {
	engine     *Engine
	ctx        context.Context
	render     routeRender
	cfg        model.Project
	scriptHash string

	logs         strings.Builder
	logTruncated bool
	conns        []*sql.DB
	txes         []*sql.Tx
	httpCount    int
	secrets      map[string]string
	secretNames  []string
	hostErr      error
	memoryBytes  int64
	simulate     bool
}

// executeJSAction runs one JS result action with project-level resource limits.
func (e *Engine) executeJSAction(render routeRender, action routeAction, actionIndex int) error {
	source := strings.TrimSpace(action.Script)
	if source == "" {
		return &actionError{Type: model.ScriptError, Err: fmt.Errorf("JS 动作缺少脚本")}
	}
	cfg := render.Project
	memoryMB := action.MemoryMB
	if memoryMB <= 0 || memoryMB > cfg.JSMaxMemoryMB {
		memoryMB = cfg.JSMaxMemoryMB
	}
	if memoryMB <= 0 {
		memoryMB = 64
	}
	timeoutMS := cfg.JSTimeoutMS
	if action.TimeoutMS > 0 && action.TimeoutMS < timeoutMS {
		timeoutMS = action.TimeoutMS
	}
	if timeoutMS <= 0 {
		timeoutMS = 5000
	}
	ctx, cancel := context.WithTimeout(e.ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	release, err := e.acquireScriptSlot(ctx, render.Project.ID, cfg.JSMaxConcurrency)
	if err != nil {
		return &actionError{Type: model.ScriptTimeout, Err: fmt.Errorf("等待脚本执行槽位超时: %w", err)}
	}
	defer release()

	run := &scriptRun{
		engine: e, ctx: ctx, render: render, cfg: cfg,
		scriptHash: scriptHash(source), secrets: map[string]string{},
	}
	started := time.Now()
	status, errType, errMsg, resultJSON := run.run(source, int64(memoryMB)<<20)
	duration := time.Since(started)
	run.closeAll()

	execution := model.ScriptExecution{
		ProjectID: render.Project.ID, AuditID: render.Audit.ID,
		TaskID: render.Task.ID, TaskNo: render.Task.TaskNo,
		ActionIndex: actionIndex,
		Remark:      truncate(action.Remark, 128), ScriptHash: run.scriptHash,
		StartedAt: started, DurationMS: duration.Milliseconds(),
		Status: status, ErrorType: errType, ErrorMessage: truncate(errMsg, 1000),
		LogText: run.logText(), MemoryBytes: run.memoryBytes,
	}
	if len(resultJSON) > 0 {
		execution.ResultJSON = model.JSONText(resultJSON)
	}
	if createErr := e.db.Create(&execution).Error; createErr != nil {
		log.Printf("script execution log: %v", createErr)
	}
	if status != model.ScriptSuccess {
		return &actionError{
			Type: errType, Err: fmt.Errorf("%s", errMsg),
			ExecutionID: execution.ID,
		}
	}
	return nil
}

func (r *scriptRun) run(source string, memoryLimit int64) (status, errType, errMsg string, result []byte) {
	status, errType = model.ScriptSuccess, ""
	defer func() {
		if recovered := recover(); recovered != nil {
			status = model.ScriptError
			errType = model.ScriptError
			errMsg = fmt.Sprintf("脚本执行 panic: %v", recovered)
			r.logf("ERROR", "%s", errMsg)
		}
	}()

	script, err := moejs.CompileScript("script.js", source)
	if err != nil {
		return model.ScriptError, model.ScriptError, err.Error(), nil
	}
	rt := moejs.NewRuntime(moejs.Options{
		MemoryLimit:        memoryLimit,
		DisableDynamicCode: true,
	})
	defer func() {
		rt.ReleaseCallData()
		rt.ClearInterrupt()
	}()

	if err := rt.SetGlobal("ctx", r.contextMap()); err != nil {
		return model.ScriptError, model.ScriptError, err.Error(), nil
	}
	if err := rt.SetGlobal("console", r.consoleMap()); err != nil {
		return model.ScriptError, model.ScriptError, err.Error(), nil
	}
	interrupted := make(chan struct{})
	stop := context.AfterFunc(r.ctx, func() {
		rt.Interrupt(context.Cause(r.ctx))
		close(interrupted)
	})
	defer func() {
		if !stop() {
			<-interrupted
		}
	}()

	// Run the script body first. Top-level code can use the global ctx directly;
	// defining main(ctx) remains supported for scripts that need an explicit return.
	completion, err := rt.RunScript(script)
	if err != nil {
		return r.classify(rt, err)
	}
	global := men.ObjectValue(rt.Realm().Global)
	mainValue, err := rt.Get(global, "main")
	if err != nil {
		return model.ScriptError, model.ScriptError, err.Error(), nil
	}
	value := completion
	if men.IsCallable(mainValue) {
		ctxValue, err := rt.Get(global, "ctx")
		if err != nil {
			return model.ScriptError, model.ScriptError, err.Error(), nil
		}
		var callErr error
		value, callErr = rt.CallFunction(mainValue, moejs.Undefined(), ctxValue)
		if callErr != nil {
			return r.classify(rt, callErr)
		}
	}
	stats := rt.Stats()
	r.memoryBytes = stats.RequestAllocatedBytes
	output, err := rt.AppendJSON(nil, value)
	if err != nil {
		return model.ScriptError, model.ScriptError, fmt.Sprintf("返回值序列化失败: %v", err), nil
	}
	if len(output) == 0 || string(output) == "null" {
		return model.ScriptSuccess, "", "", nil
	}
	if len(output) > r.cfg.JSMaxResultBytes {
		return model.ScriptHostError, model.ScriptHostError, fmt.Sprintf("脚本返回值超过 %d 字节限制", r.cfg.JSMaxResultBytes), nil
	}
	return model.ScriptSuccess, "", "", output
}

func (r *scriptRun) classify(rt *moejs.Runtime, err error) (string, string, string, []byte) {
	if errors.Is(err, moejs.ErrMemoryLimit) {
		return model.ScriptOOM, model.ScriptOOM, "脚本内存超过项目限制", nil
	}
	var interrupted *moejs.InterruptedError
	if errors.As(err, &interrupted) {
		if r.ctx.Err() == context.DeadlineExceeded {
			return model.ScriptTimeout, model.ScriptTimeout, "脚本执行超时", nil
		}
		if r.hostErr != nil {
			return model.ScriptHostError, model.ScriptHostError, r.hostErr.Error(), nil
		}
		return model.ScriptError, model.ScriptError, err.Error(), nil
	}
	var exception *moejs.Exception
	if errors.As(err, &exception) {
		if r.hostErr != nil {
			return model.ScriptHostError, model.ScriptHostError, r.hostErr.Error(), nil
		}
		message := strings.TrimSpace(rt.StackTrace(exception))
		if message == "" {
			message = exception.Error()
		}
		return model.ScriptError, model.ScriptError, message, nil
	}
	if r.ctx.Err() == context.DeadlineExceeded {
		return model.ScriptTimeout, model.ScriptTimeout, "脚本执行超时", nil
	}
	if r.hostErr != nil {
		return model.ScriptHostError, model.ScriptHostError, r.hostErr.Error(), nil
	}
	return model.ScriptError, model.ScriptError, err.Error(), nil
}

func (r *scriptRun) contextMap() map[string]any {
	var categories []model.AuditCategory
	_ = model.DecodeJSON(r.render.Audit.Categories, &categories)
	var mediaTypes []string
	_ = model.DecodeJSON(r.render.Audit.MediaTypes, &mediaTypes)
	audit := map[string]any{
		"id": r.render.Audit.ID, "project_id": r.render.Audit.ProjectID,
		"name": r.render.Audit.Name, "code": r.render.Audit.Code,
		"description":   r.render.Audit.Description,
		"decision_mode": r.render.Audit.DecisionMode, "media_types": mediaTypes,
		"categories": categories, "default_result": r.render.Audit.DefaultResult,
		"on_model_fail":     r.render.Audit.OnModelFail,
		"dedupe_window_sec": r.render.Audit.DedupeWindowSec,
		"status":            r.render.Audit.Status, "version": r.render.Audit.Version,
		"category": moejs.NativeFunc(func(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
			code, err := r.argString(realm, args, 0)
			if err != nil {
				return moejs.Undefined(), err
			}
			for _, category := range categories {
				if category.Code == code {
					return realm.FromGo(category)
				}
			}
			return moejs.Null(), nil
		}),
		"hasCategory": moejs.NativeFunc(func(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
			code, err := r.argString(realm, args, 0)
			if err != nil {
				return moejs.Undefined(), err
			}
			for _, category := range categories {
				if category.Code == code {
					return moejs.Bool(true), nil
				}
			}
			return moejs.Bool(false), nil
		}),
	}
	task := map[string]any{
		"id": r.render.Task.ID, "task_no": r.render.Task.TaskNo,
		"biz_key": r.render.Task.BizKey, "source_type": r.render.Task.SourceType,
		"source_id": r.render.Task.SourceID, "status": r.render.Task.Status,
		"review_note": r.render.Task.ReviewNote, "created_at": r.render.Task.CreatedAt,
		"input": r.render.Input,
	}
	project := map[string]any{
		"id": r.render.Project.ID, "code": r.render.Project.Code, "name": r.render.Project.Name,
	}
	data := map[string]any{
		"task": task, "audit": audit, "project": project,
		"result": r.render.FinalResult, "detail": r.render.Detail,
		"payload": routePayload(r.render), "simulation": r.simulate,
		"variables": moejs.NativeFunc(func(realm *moejs.Realm, _ moejs.Value, _ []moejs.Value) (moejs.Value, error) {
			return realm.FromGo(r.variables())
		}),
		"log":    moejs.NativeFunc(r.nativeLog),
		"secret": moejs.NativeFunc(r.nativeSecret),
		"mysql":  map[string]any{"connect": moejs.NativeFunc(r.nativeMySQLConnect)},
		"pg":     map[string]any{"connect": moejs.NativeFunc(r.nativePGConnect)},
		"http":   map[string]any{"request": moejs.NativeFunc(r.nativeHTTPRequest)},
		"crypto": map[string]any{
			"base64Encode": moejs.NativeFunc(r.nativeBase64Encode),
			"base64Decode": moejs.NativeFunc(r.nativeBase64Decode),
			"hexEncode":    moejs.NativeFunc(r.nativeHexEncode),
			"sha256":       moejs.NativeFunc(r.nativeSHA256),
			"hmacSha256":   moejs.NativeFunc(r.nativeHMACSHA256),
			"randomBytes":  moejs.NativeFunc(r.nativeRandomBytes),
			"uuid":         moejs.NativeFunc(r.nativeUUID),
		},
	}
	data["get"] = moejs.NativeFunc(func(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		path, err := r.argString(realm, args, 0)
		if err != nil {
			return moejs.Undefined(), err
		}
		value := lookupScriptPath(data, path)
		if value == nil {
			return moejs.Undefined(), nil
		}
		return realm.FromGo(value)
	})
	return data
}

func lookupScriptPath(root map[string]any, path string) any {
	var current any = root
	for _, part := range strings.Split(strings.TrimSpace(path), ".") {
		if part == "" {
			return nil
		}
		values, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = values[part]
		if !ok {
			return nil
		}
	}
	return current
}

func (r *scriptRun) consoleMap() map[string]any {
	return map[string]any{
		"log":   moejs.NativeFunc(r.nativeLog),
		"info":  moejs.NativeFunc(r.nativeLog),
		"warn":  moejs.NativeFunc(r.nativeWarn),
		"error": moejs.NativeFunc(r.nativeError),
	}
}

func (r *scriptRun) variables() map[string]any {
	return map[string]any{
		"task_no": r.render.Task.TaskNo, "task_id": r.render.Task.ID,
		"biz_key": r.render.Task.BizKey, "source_type": r.render.Task.SourceType,
		"project_code": r.render.Project.Code, "project_id": r.render.Project.ID,
		"audit_code": r.render.Audit.Code, "audit_id": r.render.Audit.ID,
		"result": r.render.FinalResult, "summary": r.render.Detail["summary"],
		"reasons": r.render.Detail["reasons"], "labels": r.render.Detail["labels"],
		"risk_level": r.render.Detail["risk_level"], "score": r.render.Detail["score"],
		"category": r.render.Detail["category"], "category_name": r.render.Detail["category_name"],
		"category_confidence": r.render.Detail["category_confidence"],
		"review_note":         r.render.Task.ReviewNote,
	}
}

func (r *scriptRun) nativeLog(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	return r.doLog(realm, "", args)
}

func (r *scriptRun) nativeWarn(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	return r.doLog(realm, "WARN", args)
}

func (r *scriptRun) nativeError(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	return r.doLog(realm, "ERROR", args)
}

func (r *scriptRun) doLog(realm *moejs.Realm, level string, args []moejs.Value) (moejs.Value, error) {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		text, err := realm.ToString(arg)
		if err != nil {
			return moejs.Undefined(), err
		}
		parts = append(parts, text.GoString())
	}
	line := strings.Join(parts, " ")
	if level != "" {
		line = "[" + level + "] " + line
	}
	r.logf("", "%s", line)
	return moejs.Undefined(), nil
}

func (r *scriptRun) nativeSecret(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	name, err := r.argString(realm, args, 0)
	if err != nil {
		return moejs.Undefined(), err
	}
	if r.simulate {
		r.logf("SIM", "模拟运行：ctx.secret(%q) 返回占位值，不读取真实密钥", name)
		return moejs.String("simulated-secret"), nil
	}
	if value, ok := r.secrets[name]; ok {
		return moejs.String(value), nil
	}
	var secret model.ProjectSecret
	err = r.engine.db.Where("project_id = ? AND name = ?", r.render.Project.ID, name).First(&secret).Error
	if err != nil {
		r.hostErr = fmt.Errorf("项目密钥 %s 不存在", name)
		return moejs.Undefined(), r.hostErr
	}
	value, err := r.engine.cipher.Decrypt(secret.ValueEnc)
	if err != nil {
		r.hostErr = fmt.Errorf("项目密钥 %s 解密失败", name)
		return moejs.Undefined(), r.hostErr
	}
	r.secrets[name] = value
	r.secretNames = append(r.secretNames, name)
	r.logf("SECRET", "读取项目密钥 %s", name)
	return moejs.String(value), nil
}

func (r *scriptRun) nativeMySQLConnect(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	if len(r.conns) >= 2 {
		r.hostErr = fmt.Errorf("每个脚本最多同时打开 2 个数据库连接")
		return moejs.Undefined(), r.hostErr
	}
	options, err := r.argMap(realm, args, 0)
	if err != nil {
		return moejs.Undefined(), err
	}
	if r.simulate {
		r.logf("SIM", "模拟运行：不连接 MySQL %v:%v/%v，query 返回空结果、exec 返回 0 行",
			options["host"], options["port"], options["database"])
		return r.simulatedDatabase(realm)
	}
	dsn, err := r.mysqlDSN(options)
	if err != nil {
		r.hostErr = err
		return moejs.Undefined(), err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		r.hostErr = err
		return moejs.Undefined(), err
	}
	return r.databaseObject(realm, db)
}

func (r *scriptRun) nativePGConnect(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	if len(r.conns) >= 2 {
		r.hostErr = fmt.Errorf("每个脚本最多同时打开 2 个数据库连接")
		return moejs.Undefined(), r.hostErr
	}
	options, err := r.argMap(realm, args, 0)
	if err != nil {
		return moejs.Undefined(), err
	}
	if r.simulate {
		r.logf("SIM", "模拟运行：不连接 PostgreSQL %v:%v/%v，query 返回空结果、exec 返回 0 行",
			options["host"], options["port"], options["database"])
		return r.simulatedDatabase(realm)
	}
	dsn, err := r.pgDSN(options)
	if err != nil {
		r.hostErr = err
		return moejs.Undefined(), err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		r.hostErr = err
		return moejs.Undefined(), err
	}
	return r.databaseObject(realm, db)
}

func (r *scriptRun) databaseObject(realm *moejs.Realm, db *sql.DB) (moejs.Value, error) {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	db.SetConnMaxLifetime(time.Minute)
	r.conns = append(r.conns, db)
	obj := realm.NewObject()
	r.setNative(realm, obj, "query", r.dbQuery(db))
	r.setNative(realm, obj, "exec", r.dbExec(db))
	r.setNative(realm, obj, "begin", r.dbBegin(db))
	r.setNative(realm, obj, "close", r.dbClose(db))
	return men.ObjectValue(obj), nil
}

func (r *scriptRun) setNative(realm *moejs.Realm, obj *men.Object, name string, fn moejs.NativeFunc) {
	value, err := realm.FromGo(fn)
	if err != nil {
		return
	}
	_ = obj.SetProp(realm, realm.KeyFromGoString(name), value)
}

func (r *scriptRun) dbQuery(db *sql.DB) moejs.NativeFunc {
	return func(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		query, err := r.argString(realm, args, 0)
		if err != nil {
			return moejs.Undefined(), err
		}
		params, err := r.argParams(realm, args, 1)
		if err != nil {
			return moejs.Undefined(), err
		}
		rows, err := db.QueryContext(r.ctx, query, params...)
		if err != nil {
			r.hostErr = err
			return moejs.Undefined(), err
		}
		defer rows.Close()
		columns, err := rows.Columns()
		if err != nil {
			r.hostErr = err
			return moejs.Undefined(), err
		}
		out := make([]map[string]any, 0, 16)
		for rows.Next() {
			if len(out) >= r.cfg.JSMaxDBRows {
				break
			}
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				r.hostErr = err
				return moejs.Undefined(), err
			}
			item := make(map[string]any, len(columns))
			for i, column := range columns {
				item[column] = scriptValue(values[i])
			}
			out = append(out, item)
		}
		if err := rows.Err(); err != nil {
			r.hostErr = err
			return moejs.Undefined(), err
		}
		return realm.FromGo(out)
	}
}

func (r *scriptRun) dbExec(db *sql.DB) moejs.NativeFunc {
	return func(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		query, err := r.argString(realm, args, 0)
		if err != nil {
			return moejs.Undefined(), err
		}
		params, err := r.argParams(realm, args, 1)
		if err != nil {
			return moejs.Undefined(), err
		}
		result, err := db.ExecContext(r.ctx, query, params...)
		if err != nil {
			r.hostErr = err
			return moejs.Undefined(), err
		}
		affected, _ := result.RowsAffected()
		lastID, _ := result.LastInsertId()
		return realm.FromGo(map[string]any{"rows_affected": affected, "last_insert_id": lastID})
	}
}

func (r *scriptRun) dbBegin(db *sql.DB) moejs.NativeFunc {
	return func(realm *moejs.Realm, _ moejs.Value, _ []moejs.Value) (moejs.Value, error) {
		tx, err := db.BeginTx(r.ctx, nil)
		if err != nil {
			r.hostErr = err
			return moejs.Undefined(), err
		}
		r.txes = append(r.txes, tx)
		obj := realm.NewObject()
		r.setNative(realm, obj, "query", r.txQuery(tx))
		r.setNative(realm, obj, "exec", r.txExec(tx))
		r.setNative(realm, obj, "commit", r.txCommit)
		r.setNative(realm, obj, "rollback", r.txRollback)
		return men.ObjectValue(obj), nil
	}
}

func (r *scriptRun) txQuery(tx *sql.Tx) moejs.NativeFunc {
	return func(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		query, err := r.argString(realm, args, 0)
		if err != nil {
			return moejs.Undefined(), err
		}
		params, err := r.argParams(realm, args, 1)
		if err != nil {
			return moejs.Undefined(), err
		}
		rows, err := tx.QueryContext(r.ctx, query, params...)
		if err != nil {
			r.hostErr = err
			return moejs.Undefined(), err
		}
		defer rows.Close()
		columns, _ := rows.Columns()
		out := make([]map[string]any, 0, 16)
		for rows.Next() {
			if len(out) >= r.cfg.JSMaxDBRows {
				break
			}
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				r.hostErr = err
				return moejs.Undefined(), err
			}
			item := make(map[string]any, len(columns))
			for i, column := range columns {
				item[column] = scriptValue(values[i])
			}
			out = append(out, item)
		}
		return realm.FromGo(out)
	}
}

func (r *scriptRun) txExec(tx *sql.Tx) moejs.NativeFunc {
	return func(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		query, err := r.argString(realm, args, 0)
		if err != nil {
			return moejs.Undefined(), err
		}
		params, err := r.argParams(realm, args, 1)
		if err != nil {
			return moejs.Undefined(), err
		}
		result, err := tx.ExecContext(r.ctx, query, params...)
		if err != nil {
			r.hostErr = err
			return moejs.Undefined(), err
		}
		affected, _ := result.RowsAffected()
		lastID, _ := result.LastInsertId()
		return realm.FromGo(map[string]any{"rows_affected": affected, "last_insert_id": lastID})
	}
}

func (r *scriptRun) txCommit(_ *moejs.Realm, _ moejs.Value, _ []moejs.Value) (moejs.Value, error) {
	if len(r.txes) == 0 {
		return moejs.Undefined(), fmt.Errorf("没有进行中的事务")
	}
	tx := r.txes[len(r.txes)-1]
	if err := tx.Commit(); err != nil {
		r.hostErr = err
		return moejs.Undefined(), err
	}
	return moejs.Undefined(), nil
}

func (r *scriptRun) txRollback(_ *moejs.Realm, _ moejs.Value, _ []moejs.Value) (moejs.Value, error) {
	if len(r.txes) == 0 {
		return moejs.Undefined(), fmt.Errorf("没有进行中的事务")
	}
	tx := r.txes[len(r.txes)-1]
	if err := tx.Rollback(); err != nil {
		r.hostErr = err
		return moejs.Undefined(), err
	}
	return moejs.Undefined(), nil
}

func (r *scriptRun) dbClose(db *sql.DB) moejs.NativeFunc {
	return func(_ *moejs.Realm, _ moejs.Value, _ []moejs.Value) (moejs.Value, error) {
		_ = db.Close()
		return moejs.Undefined(), nil
	}
}

func (r *scriptRun) nativeHTTPRequest(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	options, err := r.argMap(realm, args, 0)
	if err != nil {
		return moejs.Undefined(), err
	}
	if r.httpCount >= r.cfg.JSMaxHTTPRequests {
		r.hostErr = fmt.Errorf("脚本 HTTP 请求次数超过 %d 次限制", r.cfg.JSMaxHTTPRequests)
		return moejs.Undefined(), r.hostErr
	}
	r.httpCount++
	target := stringOption(options, "url", "")
	if target == "" {
		r.hostErr = fmt.Errorf("http.request 缺少 url")
		return moejs.Undefined(), r.hostErr
	}
	if !r.engine.cfg.AllowPrivate {
		if err := media.ValidateOutboundURL(target); err != nil {
			r.hostErr = fmt.Errorf("HTTP 地址未通过校验: %w", err)
			return moejs.Undefined(), r.hostErr
		}
	}
	method := strings.ToUpper(stringOption(options, "method", http.MethodGet))
	body := stringOption(options, "body", "")
	if r.simulate {
		r.logf("SIM", "模拟运行：拦截 HTTP %s %s，返回 200 和 {\"simulated\":true}", method, target)
		return realm.FromGo(map[string]any{
			"status": 200, "ok": true,
			"body": `{"simulated":true}`, "body_base64": "", "bytes": 17,
			"headers": map[string]string{"content-type": "application/json"},
		})
	}
	timeoutMS := intOption(options, "timeout_ms", 10000)
	if timeoutMS <= 0 || timeoutMS > 30000 {
		timeoutMS = 10000
	}
	reqCtx, cancel := context.WithTimeout(r.ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(reqCtx, method, target, strings.NewReader(body))
	if err != nil {
		r.hostErr = err
		return moejs.Undefined(), err
	}
	if headers, ok := options["headers"].(map[string]any); ok {
		for key, value := range headers {
			request.Header.Set(key, fmt.Sprint(value))
		}
	}
	client := &http.Client{
		Timeout: time.Duration(timeoutMS) * time.Millisecond,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			if !r.engine.cfg.AllowPrivate {
				if err := media.ValidateOutboundURL(req.URL.String()); err != nil {
					return err
				}
			}
			return nil
		},
	}
	response, err := client.Do(request)
	if err != nil {
		r.hostErr = err
		return moejs.Undefined(), err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, int64(r.cfg.JSMaxHTTPResponseBytes)+1))
	if err != nil {
		r.hostErr = err
		return moejs.Undefined(), err
	}
	if len(raw) > r.cfg.JSMaxHTTPResponseBytes {
		r.hostErr = fmt.Errorf("HTTP 响应超过 %d 字节限制", r.cfg.JSMaxHTTPResponseBytes)
		return moejs.Undefined(), r.hostErr
	}
	obj := realm.NewObject()
	_ = obj.SetProp(realm, realm.KeyFromGoString("status"), moejs.Int(int64(response.StatusCode)))
	_ = obj.SetProp(realm, realm.KeyFromGoString("ok"), moejs.Bool(response.StatusCode >= 200 && response.StatusCode < 300))
	_ = obj.SetProp(realm, realm.KeyFromGoString("body"), moejs.String(string(raw)))
	_ = obj.SetProp(realm, realm.KeyFromGoString("body_base64"), moejs.String(base64.StdEncoding.EncodeToString(raw)))
	headerValue, err := realm.FromGo(response.Header)
	if err == nil {
		_ = obj.SetProp(realm, realm.KeyFromGoString("headers"), headerValue)
	}
	if err := realm.ChargeMemory(len(raw)); err == nil {
		if buffer, bufferErr := realm.NewArrayBuffer(raw); bufferErr == nil {
			_ = obj.SetProp(realm, realm.KeyFromGoString("bytes"), men.ObjectValue(buffer))
		}
	}
	return men.ObjectValue(obj), nil
}

func (r *scriptRun) nativeBase64Encode(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	value, err := r.argString(realm, args, 0)
	if err != nil {
		return moejs.Undefined(), err
	}
	return moejs.String(base64.StdEncoding.EncodeToString([]byte(value))), nil
}

func (r *scriptRun) nativeBase64Decode(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	value, err := r.argString(realm, args, 0)
	if err != nil {
		return moejs.Undefined(), err
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return moejs.Undefined(), err
	}
	if err := realm.ChargeMemory(len(decoded)); err != nil {
		return moejs.Undefined(), err
	}
	buffer, err := realm.NewArrayBuffer(decoded)
	if err != nil {
		return moejs.Undefined(), err
	}
	return men.ObjectValue(buffer), nil
}

func (r *scriptRun) nativeHexEncode(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	value, err := r.argString(realm, args, 0)
	if err != nil {
		return moejs.Undefined(), err
	}
	return moejs.String(hex.EncodeToString([]byte(value))), nil
}

func (r *scriptRun) nativeSHA256(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	value, err := r.argString(realm, args, 0)
	if err != nil {
		return moejs.Undefined(), err
	}
	sum := sha256.Sum256([]byte(value))
	return moejs.String(hex.EncodeToString(sum[:])), nil
}

func (r *scriptRun) nativeHMACSHA256(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	key, err := r.argString(realm, args, 0)
	if err != nil {
		return moejs.Undefined(), err
	}
	value, err := r.argString(realm, args, 1)
	if err != nil {
		return moejs.Undefined(), err
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(value))
	return moejs.String(hex.EncodeToString(mac.Sum(nil))), nil
}

func (r *scriptRun) nativeRandomBytes(realm *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	size := 16
	if len(args) > 0 {
		number, err := realm.ToNumber(args[0])
		if err == nil && number > 0 && number <= 1024 {
			size = int(number)
		}
	}
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return moejs.Undefined(), err
	}
	if err := realm.ChargeMemory(len(data)); err != nil {
		return moejs.Undefined(), err
	}
	buffer, err := realm.NewArrayBuffer(data)
	if err != nil {
		return moejs.Undefined(), err
	}
	return men.ObjectValue(buffer), nil
}

func (r *scriptRun) nativeUUID(_ *moejs.Realm, _ moejs.Value, _ []moejs.Value) (moejs.Value, error) {
	return moejs.String(uuid.NewString()), nil
}

func (r *scriptRun) mysqlDSN(options map[string]any) (string, error) {
	host := stringOption(options, "host", "")
	port := intOption(options, "port", 3306)
	user := stringOption(options, "user", "")
	password := stringOption(options, "password", "")
	database := stringOption(options, "database", "")
	if host == "" || database == "" {
		return "", fmt.Errorf("mysql.connect 需要 host 和 database")
	}
	if err := r.validateHost(host, port); err != nil {
		return "", err
	}
	cfg := mysql.Config{
		User: user, Passwd: password, Net: "tcp",
		Addr: net.JoinHostPort(host, strconv.Itoa(port)), DBName: database,
		Params:  map[string]string{"charset": "utf8mb4", "parseTime": "true"},
		Timeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second,
	}
	if tlsMode := stringOption(options, "tls", ""); tlsMode != "" {
		cfg.TLSConfig = tlsMode
	}
	return cfg.FormatDSN(), nil
}

func (r *scriptRun) pgDSN(options map[string]any) (string, error) {
	host := stringOption(options, "host", "")
	port := intOption(options, "port", 5432)
	user := stringOption(options, "user", "")
	password := stringOption(options, "password", "")
	database := stringOption(options, "database", "")
	if host == "" || database == "" {
		return "", fmt.Errorf("pg.connect 需要 host 和 database")
	}
	if err := r.validateHost(host, port); err != nil {
		return "", err
	}
	sslmode := stringOption(options, "sslmode", "disable")
	target := url.URL{
		Scheme: "postgres", User: url.UserPassword(user, password),
		Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: "/" + database,
	}
	query := target.Query()
	query.Set("sslmode", sslmode)
	query.Set("connect_timeout", "5")
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func (r *scriptRun) validateHost(host string, port int) error {
	host = strings.TrimSpace(host)
	if host == "" || port <= 0 || port > 65535 {
		return fmt.Errorf("数据库地址无效")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("不允许连接链路本地或未指定地址")
		}
	}
	if !r.engine.cfg.AllowPrivate {
		target := "http://" + net.JoinHostPort(host, strconv.Itoa(port))
		if err := media.ValidateOutboundURL(target); err != nil {
			return fmt.Errorf("数据库地址未通过校验: %w", err)
		}
	}
	return nil
}

func (r *scriptRun) argString(realm *moejs.Realm, args []moejs.Value, index int) (string, error) {
	value := moejs.Arg(args, index)
	text, err := realm.ToString(value)
	if err != nil {
		return "", err
	}
	return text.GoString(), nil
}

func (r *scriptRun) argMap(realm *moejs.Realm, args []moejs.Value, index int) (map[string]any, error) {
	value := moejs.Arg(args, index)
	converted, err := realm.ToGoStrict(value)
	if err != nil {
		return nil, err
	}
	options, ok := converted.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("参数必须是对象")
	}
	return options, nil
}

func (r *scriptRun) argParams(realm *moejs.Realm, args []moejs.Value, index int) ([]any, error) {
	value := moejs.Arg(args, index)
	if value == moejs.Undefined() || value == moejs.Null() {
		return nil, nil
	}
	converted, err := realm.ToGoStrict(value)
	if err != nil {
		return nil, err
	}
	items, ok := converted.([]any)
	if !ok {
		return nil, fmt.Errorf("SQL 参数必须是数组")
	}
	return items, nil
}

func (r *scriptRun) logf(level, format string, args ...any) {
	if r.logTruncated {
		return
	}
	line := fmt.Sprintf(format, args...)
	if level != "" {
		line = "[" + level + "] " + line
	}
	line = fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02 15:04:05.000"), line)
	if r.logs.Len()+len(line) > r.cfg.JSMaxLogBytes {
		remaining := r.cfg.JSMaxLogBytes - r.logs.Len()
		if remaining > 0 {
			r.logs.WriteString(line[:remaining])
		}
		r.logs.WriteString("...[日志已截断]\n")
		r.logTruncated = true
		return
	}
	r.logs.WriteString(line)
}

func (r *scriptRun) logText() string {
	if len(r.secretNames) > 0 {
		r.logf("SECRET", "本次脚本读取的密钥: %s", strings.Join(r.secretNames, ", "))
	}
	return r.logs.String()
}

func (r *scriptRun) closeAll() {
	for i := len(r.txes) - 1; i >= 0; i-- {
		_ = r.txes[i].Rollback()
	}
	for _, db := range r.conns {
		_ = db.Close()
	}
}

func (e *Engine) acquireScriptSlot(ctx context.Context, projectID uint64, limit int) (func(), error) {
	if limit <= 0 {
		limit = 2
	}
	e.scriptMu.Lock()
	if e.scriptSlots == nil {
		e.scriptSlots = map[uint64]chan struct{}{}
	}
	slot, ok := e.scriptSlots[projectID]
	if !ok || cap(slot) != limit {
		slot = make(chan struct{}, limit)
		e.scriptSlots[projectID] = slot
	}
	e.scriptMu.Unlock()
	select {
	case slot <- struct{}{}:
		return func() { <-slot }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func scriptHash(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func scriptValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return base64.StdEncoding.EncodeToString(typed)
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	default:
		return typed
	}
}

func stringOption(options map[string]any, key, fallback string) string {
	value, ok := options[key]
	if !ok || value == nil {
		return fallback
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return fallback
	}
	return text
}

func intOption(options map[string]any, key string, fallback int) int {
	value, ok := options[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		number, err := typed.Int64()
		if err == nil {
			return int(number)
		}
	case string:
		number, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return number
		}
	}
	return fallback
}
