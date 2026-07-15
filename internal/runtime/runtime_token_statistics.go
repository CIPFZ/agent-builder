package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	database "github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/message"
)

type RuntimeContextStatisticsRequest struct {
	From     string `json:"from"`
	To       string `json:"to"`
	View     string `json:"view"`
	Timezone string `json:"timezone"`
}

type RuntimeContextStatisticsPoint struct {
	Day                 string `json:"day"`
	Timezone            string `json:"timezone"`
	InputTokens         string `json:"inputTokens"`
	OutputTokens        string `json:"outputTokens"`
	CacheReadTokens     string `json:"cacheReadTokens"`
	CacheCreationTokens string `json:"cacheCreationTokens"`
	ReasoningTokens     string `json:"reasoningTokens"`
	TotalTokens         string `json:"totalTokens"`
	SessionCount        string `json:"sessionCount"`
	TurnCount           string `json:"turnCount"`
	ModelCallCount      string `json:"modelCallCount"`
}

type RuntimeContextStatistics struct {
	TotalTokens         string                          `json:"totalTokens"`
	InputTokens         string                          `json:"inputTokens"`
	OutputTokens        string                          `json:"outputTokens"`
	CacheReadTokens     string                          `json:"cacheReadTokens"`
	CacheCreationTokens string                          `json:"cacheCreationTokens"`
	ReasoningTokens     string                          `json:"reasoningTokens"`
	ModelCallCount      string                          `json:"modelCallCount"`
	PeakTokens          string                          `json:"peakTokens"`
	PeakAt              string                          `json:"peakAt"`
	LongestTurnMillis   string                          `json:"longestTurnMillis"`
	LongestTurnID       string                          `json:"longestTurnId"`
	CurrentStreakDays   string                          `json:"currentStreakDays"`
	LongestStreakDays   string                          `json:"longestStreakDays"`
	ActiveDays          string                          `json:"activeDays"`
	LastUpdatedAt       string                          `json:"lastUpdatedAt"`
	Points              []RuntimeContextStatisticsPoint `json:"points"`
}

type tokenUsageDelta struct {
	input, output, cacheRead, cacheCreation, reasoning, total, sessions, turns, calls int64
	peak, peakAt                                                                      int64
}

func addUsageDelta(left, right tokenUsageDelta) (tokenUsageDelta, error) {
	var err error
	for _, item := range []struct {
		target *int64
		value  int64
	}{{&left.input, right.input}, {&left.output, right.output}, {&left.cacheRead, right.cacheRead}, {&left.cacheCreation, right.cacheCreation}, {&left.reasoning, right.reasoning}, {&left.total, right.total}, {&left.sessions, right.sessions}, {&left.turns, right.turns}, {&left.calls, right.calls}} {
		*item.target, err = checkedAdd(*item.target, item.value)
		if err != nil {
			return tokenUsageDelta{}, err
		}
	}
	if right.peak > left.peak {
		left.peak, left.peakAt = right.peak, right.peakAt
	}
	return left, nil
}

func checkedAdd(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, errors.New("token statistics int64 overflow")
	}
	return a + b, nil
}
func parseDecimal(v any) (int64, error) {
	s, ok := v.(string)
	if !ok {
		return 0, errors.New("usage value must be decimal string")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid usage decimal %q", s)
	}
	return n, err
}
func usageDeltaFromPayload(raw string) (tokenUsageDelta, bool, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return tokenUsageDelta{}, false, err
	}
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		return tokenUsageDelta{}, false, nil
	}
	vals := make([]*int64, 5)
	for i, key := range []string{"input", "output", "cacheRead", "cacheCreation", "reasoning"} {
		if v, exists := usage[key]; exists {
			n, err := parseDecimal(v)
			if err != nil {
				return tokenUsageDelta{}, false, err
			}
			vals[i] = &n
		} else {
			z := int64(0)
			vals[i] = &z
		}
	}
	d := tokenUsageDelta{input: *vals[0], output: *vals[1], cacheRead: *vals[2], cacheCreation: *vals[3], reasoning: *vals[4], calls: 1}
	for _, n := range []int64{d.input, d.output, d.cacheRead, d.cacheCreation} {
		var err error
		d.total, err = checkedAdd(d.total, n)
		if err != nil {
			return tokenUsageDelta{}, false, err
		}
	}
	d.peak = d.total
	return d, true, nil
}

type tokenStatisticsStore struct {
	db *sql.DB
	mu sync.Mutex
}

func (s *tokenStatisticsStore) repairEmptyProjection(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var expected tokenUsageDelta
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(COALESCE(json_extract(usage_json,'$.input'),0)),0),COALESCE(SUM(COALESCE(json_extract(usage_json,'$.output'),0)),0),COALESCE(SUM(COALESCE(json_extract(usage_json,'$.cacheRead'),0)),0),COALESCE(SUM(COALESCE(json_extract(usage_json,'$.cacheCreation'),0)),0),COALESCE(SUM(COALESCE(json_extract(usage_json,'$.reasoning'),0)),0) FROM messages WHERE role='assistant' AND usage_json IS NOT NULL AND usage_json <> '' AND (COALESCE(json_extract(usage_json,'$.input'),0) + COALESCE(json_extract(usage_json,'$.output'),0) + COALESCE(json_extract(usage_json,'$.cacheRead'),0) + COALESCE(json_extract(usage_json,'$.cacheCreation'),0)) > 0`).Scan(&expected.calls, &expected.input, &expected.output, &expected.cacheRead, &expected.cacheCreation, &expected.reasoning); err != nil {
		return err
	}
	for _, value := range []int64{expected.input, expected.output, expected.cacheRead, expected.cacheCreation} {
		var err error
		expected.total, err = checkedAdd(expected.total, value)
		if err != nil {
			return err
		}
	}
	var actual tokenUsageDelta
	if err := s.db.QueryRowContext(ctx, `SELECT input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,reasoning_tokens,total_tokens,model_call_count FROM runtime_token_usage_lifetime WHERE id=1`).Scan(&actual.input, &actual.output, &actual.cacheRead, &actual.cacheCreation, &actual.reasoning, &actual.total, &actual.calls); err != nil {
		return err
	}
	if expected.input == actual.input && expected.output == actual.output && expected.cacheRead == actual.cacheRead && expected.cacheCreation == actual.cacheCreation && expected.reasoning == actual.reasoning && expected.total == actual.total && expected.calls == actual.calls {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM runtime_token_usage_daily`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runtime_token_usage_lifetime SET input_tokens=0,output_tokens=0,cache_read_tokens=0,cache_creation_tokens=0,reasoning_tokens=0,total_tokens=0,model_call_count=0,peak_tokens=0,peak_at=0,updated_at=? WHERE id=1`, time.Now().UnixMilli()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE token_statistics_cursor SET sequence=0,backfilled=0,updated_at=? WHERE id=1`, time.Now().UnixMilli()); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.backfill(ctx)
}

func messageCreatedAtMillis(createdAt int64) int64 {
	const unixSecondsUpperBound = int64(100_000_000_000)
	if createdAt > -unixSecondsUpperBound && createdAt < unixSecondsUpperBound {
		return createdAt * int64(time.Second/time.Millisecond)
	}
	return createdAt
}

func (s *tokenStatisticsStore) backfill(ctx context.Context) error {
	var done int64
	if err := s.db.QueryRowContext(ctx, `SELECT backfilled FROM token_statistics_cursor WHERE id=1`).Scan(&done); err != nil {
		return err
	}
	if done != 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT created_at, session_id, usage_json FROM messages WHERE role='assistant' AND usage_json IS NOT NULL AND usage_json <> '' ORDER BY created_at`)
	if err != nil {
		return err
	}
	lifetime := tokenUsageDelta{}
	days := map[string]tokenUsageDelta{}
	sessionsByDay := map[string]map[string]struct{}{}
	loc := time.UTC
	for rows.Next() {
		var created int64
		var sid, raw string
		if err := rows.Scan(&created, &sid, &raw); err != nil {
			return err
		}
		var u message.Usage
		if json.Unmarshal([]byte(raw), &u) != nil || u.IsZero() {
			continue
		}
		createdAt := messageCreatedAtMillis(created)
		d := tokenUsageDelta{input: u.InputTokens, output: u.OutputTokens, cacheRead: u.CacheReadTokens, cacheCreation: u.CacheCreationTokens, reasoning: u.ReasoningTokens, calls: 1, peakAt: createdAt}
		for _, value := range []int64{d.input, d.output, d.cacheRead, d.cacheCreation, d.reasoning} {
			if value < 0 {
				return errors.New("negative usage")
			}
		}
		for _, value := range []int64{d.input, d.output, d.cacheRead, d.cacheCreation} {
			d.total, err = checkedAdd(d.total, value)
			if err != nil {
				return err
			}
		}
		d.peak = d.total
		day := time.UnixMilli(createdAt).In(loc).Format("2006-01-02")
		days[day], err = addUsageDelta(days[day], d)
		if err != nil {
			return err
		}
		if sessionsByDay[day] == nil {
			sessionsByDay[day] = map[string]struct{}{}
		}
		sessionsByDay[day][sid] = struct{}{}
		lifetime, err = addUsageDelta(lifetime, d)
		if err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for day, d := range days {
		if _, err = tx.ExecContext(ctx, `INSERT INTO runtime_token_usage_daily(day,timezone,input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,reasoning_tokens,session_count,turn_count,model_call_count,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(day) DO UPDATE SET timezone=excluded.timezone,input_tokens=excluded.input_tokens,output_tokens=excluded.output_tokens,cache_read_tokens=excluded.cache_read_tokens,cache_creation_tokens=excluded.cache_creation_tokens,reasoning_tokens=excluded.reasoning_tokens,session_count=excluded.session_count,turn_count=excluded.turn_count,model_call_count=excluded.model_call_count,updated_at=excluded.updated_at`, day, "UTC", d.input, d.output, d.cacheRead, d.cacheCreation, d.reasoning, int64(len(sessionsByDay[day])), 0, d.calls, now); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runtime_token_usage_lifetime SET input_tokens=?,output_tokens=?,cache_read_tokens=?,cache_creation_tokens=?,reasoning_tokens=?,total_tokens=?,model_call_count=?,peak_tokens=?,peak_at=?,updated_at=? WHERE id=1`, lifetime.input, lifetime.output, lifetime.cacheRead, lifetime.cacheCreation, lifetime.reasoning, lifetime.total, lifetime.calls, lifetime.peak, lifetime.peakAt, now); err != nil {
		return err
	}
	var max sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT MAX(sequence) FROM runtime_events`).Scan(&max); err != nil {
		return err
	}
	seq := int64(0)
	if max.Valid {
		seq = max.Int64
	}
	if _, err = tx.ExecContext(ctx, `UPDATE token_statistics_cursor SET sequence=?,backfilled=1,updated_at=? WHERE id=1`, seq, now); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *tokenStatisticsStore) consume(ctx context.Context, limit int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cursor int64
	if err := s.db.QueryRowContext(ctx, `SELECT sequence FROM token_statistics_cursor WHERE id=1`).Scan(&cursor); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT sequence,type,session_id,turn_id,created_at,payload_json FROM runtime_events WHERE sequence>? ORDER BY sequence LIMIT ?`, cursor, limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	type event struct {
		seq               int64
		eventType         string
		sessionID, turnID sql.NullString
		at, payload       string
	}
	list := []event{}
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.seq, &e.eventType, &e.sessionID, &e.turnID, &e.at, &e.payload); err != nil {
			return err
		}
		list = append(list, e)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(list) == 0 {
		return nil
	}
	days := map[string]tokenUsageDelta{}
	sessionsByDay := map[string]map[string]struct{}{}
	turnsByDay := map[string]map[string]struct{}{}
	last := cursor
	for _, e := range list {
		last = e.seq
		if e.eventType != "message.completed" {
			continue
		}
		d, ok, err := usageDeltaFromPayload(e.payload)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, e.at)
		if err != nil {
			continue
		}
		day := t.UTC().Format("2006-01-02")
		d.peakAt = t.UnixMilli()
		days[day], err = addUsageDelta(days[day], d)
		if err != nil {
			return err
		}
		if e.sessionID.Valid {
			if sessionsByDay[day] == nil {
				sessionsByDay[day] = map[string]struct{}{}
			}
			sessionsByDay[day][e.sessionID.String] = struct{}{}
		}
		if e.turnID.Valid {
			if turnsByDay[day] == nil {
				turnsByDay[day] = map[string]struct{}{}
			}
			turnsByDay[day][e.turnID.String] = struct{}{}
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	for day, d := range days {
		var current tokenUsageDelta
		_ = tx.QueryRowContext(ctx, `SELECT input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,reasoning_tokens,session_count,turn_count,model_call_count FROM runtime_token_usage_daily WHERE day=?`, day).Scan(&current.input, &current.output, &current.cacheRead, &current.cacheCreation, &current.reasoning, &current.sessions, &current.turns, &current.calls)
		d.sessions = int64(len(sessionsByDay[day]))
		d.turns = int64(len(turnsByDay[day]))
		if _, err = addUsageDelta(current, d); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO runtime_token_usage_daily(day,timezone,input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,reasoning_tokens,session_count,turn_count,model_call_count,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(day) DO UPDATE SET input_tokens=input_tokens+excluded.input_tokens,output_tokens=output_tokens+excluded.output_tokens,cache_read_tokens=cache_read_tokens+excluded.cache_read_tokens,cache_creation_tokens=cache_creation_tokens+excluded.cache_creation_tokens,reasoning_tokens=reasoning_tokens+excluded.reasoning_tokens,session_count=session_count+excluded.session_count,turn_count=turn_count+excluded.turn_count,model_call_count=model_call_count+excluded.model_call_count,updated_at=excluded.updated_at`, day, "UTC", d.input, d.output, d.cacheRead, d.cacheCreation, d.reasoning, d.sessions, d.turns, d.calls, now); err != nil {
			return err
		}
	}
	batch := tokenUsageDelta{}
	for _, d := range days {
		batch, err = addUsageDelta(batch, d)
		if err != nil {
			return err
		}
	}
	var current tokenUsageDelta
	if err = tx.QueryRowContext(ctx, `SELECT input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,reasoning_tokens,total_tokens,model_call_count,peak_tokens,peak_at FROM runtime_token_usage_lifetime WHERE id=1`).Scan(&current.input, &current.output, &current.cacheRead, &current.cacheCreation, &current.reasoning, &current.total, &current.calls, &current.peak, &current.peakAt); err != nil {
		return err
	}
	next, err := addUsageDelta(current, batch)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runtime_token_usage_lifetime SET input_tokens=?,output_tokens=?,cache_read_tokens=?,cache_creation_tokens=?,reasoning_tokens=?,total_tokens=?,model_call_count=?,peak_tokens=?,peak_at=?,updated_at=? WHERE id=1`, next.input, next.output, next.cacheRead, next.cacheCreation, next.reasoning, next.total, next.calls, next.peak, next.peakAt, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE token_statistics_cursor SET sequence=?,updated_at=? WHERE id=1`, last, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM runtime_token_usage_daily WHERE day < date('now','-365 day')`); err != nil {
		return err
	}
	return tx.Commit()
}
func sum(days map[string]tokenUsageDelta, field string) int64 {
	var n int64
	for _, d := range days {
		switch field {
		case "input":
			n += d.input
		case "output":
			n += d.output
		case "cacheRead":
			n += d.cacheRead
		case "cacheCreation":
			n += d.cacheCreation
		case "reasoning":
			n += d.reasoning
		case "calls":
			n += d.calls
		}
	}
	return n
}
func (r *runtimeService) startTokenStatistics(ctx context.Context, db *sql.DB) error {
	store := &tokenStatisticsStore{db: db}
	if err := store.backfill(ctx); err != nil {
		return fmt.Errorf("failed to backfill token statistics: %w", err)
	}
	if err := store.repairEmptyProjection(ctx); err != nil {
		return fmt.Errorf("failed to repair token statistics: %w", err)
	}
	r.mu.Lock()
	r.tokenStatistics = store
	r.mu.Unlock()
	go func() {
		nextDelay := func(first bool) time.Duration {
			if first {
				return 60*time.Second + time.Duration(time.Now().UnixNano()%int64(61*time.Second))
			}
			return 10*time.Minute + time.Duration(time.Now().UnixNano()%int64(5*time.Minute))
		}
		timer := time.NewTimer(nextDelay(true))
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
				started := time.Now()
				for batch := 0; batch < 4 && time.Since(started) < 40*time.Millisecond; batch++ {
					if err := store.consume(ctx, 256); err != nil {
						break
					}
				}
				timer.Reset(nextDelay(false))
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}

func (r *runtimeService) ContextStatistics(ctx context.Context, req RuntimeContextStatisticsRequest) (RuntimeContextStatistics, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeContextStatistics{}, err
	}
	dataDir := cfg.Config().Options.DataDirectory
	conn, err := database.Connect(ctx, dataDir)
	if err != nil {
		return RuntimeContextStatistics{}, err
	}
	defer func() { _ = database.Release(dataDir) }()
	db := conn
	r.mu.Lock()
	store := r.tokenStatistics
	r.mu.Unlock()
	if store != nil {
		if err := store.consume(ctx, 256); err != nil {
			return RuntimeContextStatistics{}, fmt.Errorf("failed to update token statistics: %w", err)
		}
		if err := store.repairEmptyProjection(ctx); err != nil {
			return RuntimeContextStatistics{}, fmt.Errorf("failed to repair token statistics: %w", err)
		}
	}
	tz := strings.TrimSpace(req.Timezone)
	loc := time.UTC
	if tz != "" {
		parsed, e := time.LoadLocation(tz)
		if e != nil {
			return RuntimeContextStatistics{}, fmt.Errorf("invalid IANA timezone %q", tz)
		}
		loc = parsed
	}
	if req.From == "" {
		req.From = time.Now().In(loc).AddDate(0, 0, -364).Format("2006-01-02")
	}
	if req.To == "" {
		req.To = time.Now().In(loc).Format("2006-01-02")
	}
	rows, err := db.QueryContext(ctx, `SELECT day,timezone,input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,reasoning_tokens,session_count,turn_count,model_call_count FROM runtime_token_usage_daily WHERE day>=? AND day<=? ORDER BY day`, req.From, req.To)
	if err != nil {
		return RuntimeContextStatistics{}, err
	}
	defer rows.Close()
	out := RuntimeContextStatistics{Points: []RuntimeContextStatisticsPoint{}}
	for rows.Next() {
		var p RuntimeContextStatisticsPoint
		var in, outp, cr, cc, reas, sess, turn, calls int64
		if err := rows.Scan(&p.Day, &p.Timezone, &in, &outp, &cr, &cc, &reas, &sess, &turn, &calls); err != nil {
			return out, err
		}
		p.InputTokens = strconv.FormatInt(in, 10)
		p.OutputTokens = strconv.FormatInt(outp, 10)
		p.CacheReadTokens = strconv.FormatInt(cr, 10)
		p.CacheCreationTokens = strconv.FormatInt(cc, 10)
		p.ReasoningTokens = strconv.FormatInt(reas, 10)
		p.TotalTokens = strconv.FormatInt(in+outp+cr+cc, 10)
		p.SessionCount = strconv.FormatInt(sess, 10)
		p.TurnCount = strconv.FormatInt(turn, 10)
		p.ModelCallCount = strconv.FormatInt(calls, 10)
		out.Points = append(out.Points, p)
	}
	var in, outp, cr, cc, reas, total, calls, peak, peakAt, updated int64
	if err = db.QueryRowContext(ctx, `SELECT input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,reasoning_tokens,total_tokens,model_call_count,peak_tokens,peak_at,updated_at FROM runtime_token_usage_lifetime WHERE id=1`).Scan(&in, &outp, &cr, &cc, &reas, &total, &calls, &peak, &peakAt, &updated); err != nil {
		return out, err
	}
	out.TotalTokens = strconv.FormatInt(total, 10)
	out.InputTokens = strconv.FormatInt(in, 10)
	out.OutputTokens = strconv.FormatInt(outp, 10)
	out.CacheReadTokens = strconv.FormatInt(cr, 10)
	out.CacheCreationTokens = strconv.FormatInt(cc, 10)
	out.ReasoningTokens = strconv.FormatInt(reas, 10)
	out.ModelCallCount = strconv.FormatInt(calls, 10)
	out.PeakTokens = strconv.FormatInt(peak, 10)
	if peakAt > 0 {
		out.PeakAt = time.UnixMilli(peakAt).UTC().Format(time.RFC3339Nano)
	}
	out.LastUpdatedAt = time.UnixMilli(updated).UTC().Format(time.RFC3339Nano)
	var longest int64
	var longestID sql.NullString
	_ = db.QueryRowContext(ctx, `SELECT id,finished_at-started_at FROM runtime_turns WHERE finished_at IS NOT NULL ORDER BY finished_at-started_at DESC LIMIT 1`).Scan(&longestID, &longest)
	out.LongestTurnMillis = strconv.FormatInt(longest, 10)
	out.LongestTurnID = longestID.String
	current, longestStreak := statisticsStreaks(out.Points, time.Now().In(loc))
	out.CurrentStreakDays = strconv.FormatInt(current, 10)
	out.LongestStreakDays = strconv.FormatInt(longestStreak, 10)
	out.ActiveDays = strconv.Itoa(len(out.Points))
	return out, nil
}

func statisticsStreaks(points []RuntimeContextStatisticsPoint, now time.Time) (int64, int64) {
	active := map[string]bool{}
	for _, p := range points {
		calls, _ := strconv.ParseInt(p.ModelCallCount, 10, 64)
		if calls > 0 {
			active[p.Day] = true
		}
	}
	days := make([]string, 0, len(active))
	for day := range active {
		days = append(days, day)
	}
	sort.Strings(days)
	var longest, currentRun int64
	var previous time.Time
	for _, day := range days {
		parsed, _ := time.Parse("2006-01-02", day)
		if !previous.IsZero() && parsed.Sub(previous) == 24*time.Hour {
			currentRun++
		} else {
			currentRun = 1
		}
		if currentRun > longest {
			longest = currentRun
		}
		previous = parsed
	}
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	anchor := today
	if !active[today] {
		anchor = yesterday
	}
	var current int64
	for active[anchor] {
		current++
		parsed, _ := time.Parse("2006-01-02", anchor)
		anchor = parsed.AddDate(0, 0, -1).Format("2006-01-02")
	}
	return current, longest
}
