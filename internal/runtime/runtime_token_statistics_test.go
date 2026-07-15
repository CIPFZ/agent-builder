package runtime

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/db"
)

func TestUsageDeltaRequiresDecimalStringsAndDoesNotDoubleReasoning(t *testing.T) {
	delta, ok, err := usageDeltaFromPayload(`{"usage":{"input":"10","output":"20","cacheRead":"3","cacheCreation":"4","reasoning":"7"}}`)
	if err != nil || !ok {
		t.Fatalf("delta=%#v ok=%v err=%v", delta, ok, err)
	}
	if delta.total != 37 {
		t.Fatalf("total=%d", delta.total)
	}
	if _, _, err := usageDeltaFromPayload(`{"usage":{"input":10}}`); err == nil {
		t.Fatal("expected JSON number rejection")
	}
}

func TestTokenStatisticsCursorAdvancesUnrelatedAndCountsOnce(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })
	store := &tokenStatisticsStore{db: conn}
	if _, err = conn.ExecContext(ctx, `INSERT INTO runtime_events(sequence,id,type,payload_json,created_at) VALUES(1,'other','turn.completed','{}','2026-01-01T00:00:00Z'),(2,'message','message.completed','{"usage":{"input":"10","output":"20","cacheRead":"3","cacheCreation":"4","reasoning":"7"}}','2026-01-01T00:00:01Z')`); err != nil {
		t.Fatal(err)
	}
	if err = store.consume(ctx, 128); err != nil {
		t.Fatal(err)
	}
	if err = store.consume(ctx, 128); err != nil {
		t.Fatal(err)
	}
	var total, calls, cursor int64
	if err = conn.QueryRowContext(ctx, `SELECT total_tokens,model_call_count FROM runtime_token_usage_lifetime WHERE id=1`).Scan(&total, &calls); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRowContext(ctx, `SELECT sequence FROM token_statistics_cursor WHERE id=1`).Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	if total != 37 || calls != 1 || cursor != 2 {
		t.Fatalf("total=%d calls=%d cursor=%d", total, calls, cursor)
	}
}

func TestCheckedAddRejectsInt64Overflow(t *testing.T) {
	if _, err := checkedAdd(math.MaxInt64, 1); err == nil {
		t.Fatal("expected overflow")
	}
}

func TestRepairEmptyProjectionBackfillsSecondTimestamps(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })
	createdAt := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC).Unix()
	if _, err = conn.ExecContext(ctx, `INSERT INTO sessions(id,title,scope,workdir,updated_at,created_at) VALUES('session','test','standalone','C:/work',?,?)`, createdAt, createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `INSERT INTO messages(id,session_id,role,parts,created_at,updated_at,usage_json) VALUES('assistant','session','assistant','[]',?,?,?)`, createdAt, createdAt, `{"input":10,"output":20,"cacheRead":3,"cacheCreation":4,"reasoning":7}`); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `INSERT INTO runtime_events(sequence,id,type,payload_json,created_at) VALUES(10,'old-event','turn.completed','{}','2026-07-15T12:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `UPDATE token_statistics_cursor SET sequence=10,backfilled=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	store := &tokenStatisticsStore{db: conn}
	if err = store.repairEmptyProjection(ctx); err != nil {
		t.Fatal(err)
	}
	var total, calls, peakAt, cursor int64
	if err = conn.QueryRowContext(ctx, `SELECT total_tokens,model_call_count,peak_at FROM runtime_token_usage_lifetime WHERE id=1`).Scan(&total, &calls, &peakAt); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRowContext(ctx, `SELECT sequence FROM token_statistics_cursor WHERE id=1`).Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	var day string
	if err = conn.QueryRowContext(ctx, `SELECT day FROM runtime_token_usage_daily`).Scan(&day); err != nil {
		t.Fatal(err)
	}
	if total != 37 || calls != 1 || peakAt != createdAt*1000 || cursor != 10 || day != "2026-07-15" {
		t.Fatalf("total=%d calls=%d peakAt=%d cursor=%d day=%q", total, calls, peakAt, cursor, day)
	}
}

func TestRepairProjectionRebuildsWhenCompletedEventWasMissing(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })
	createdAt := time.Date(2026, time.July, 15, 13, 10, 43, 0, time.UTC).Unix()
	if _, err = conn.ExecContext(ctx, `INSERT INTO sessions(id,title,scope,workdir,updated_at,created_at) VALUES('session','test','standalone','C:/work',?,?)`, createdAt, createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `INSERT INTO messages(id,session_id,role,parts,created_at,updated_at,usage_json) VALUES('assistant-old','session','assistant','[]',?,?,?),('assistant-new','session','assistant','[]',?,?,?)`, createdAt-60, createdAt-60, `{"input":0,"output":10,"cacheRead":0,"cacheCreation":0,"reasoning":0}`, createdAt, createdAt, `{"input":100,"output":20,"cacheRead":5,"cacheCreation":2,"reasoning":3}`); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `UPDATE runtime_token_usage_lifetime SET output_tokens=10,total_tokens=10,model_call_count=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `INSERT INTO runtime_token_usage_daily(day,timezone,output_tokens,model_call_count,updated_at) VALUES('2026-07-15','UTC',10,1,?)`, createdAt*1000); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `UPDATE token_statistics_cursor SET sequence=99,backfilled=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	store := &tokenStatisticsStore{db: conn}
	if err = store.repairEmptyProjection(ctx); err != nil {
		t.Fatal(err)
	}
	var input, output, cacheRead, cacheCreation, total, calls int64
	if err = conn.QueryRowContext(ctx, `SELECT input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,total_tokens,model_call_count FROM runtime_token_usage_lifetime WHERE id=1`).Scan(&input, &output, &cacheRead, &cacheCreation, &total, &calls); err != nil {
		t.Fatal(err)
	}
	if input != 100 || output != 30 || cacheRead != 5 || cacheCreation != 2 || total != 137 || calls != 2 {
		t.Fatalf("input=%d output=%d cacheRead=%d cacheCreation=%d total=%d calls=%d", input, output, cacheRead, cacheCreation, total, calls)
	}
}
