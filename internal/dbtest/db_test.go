package dbtest_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
	"github.com/uptrace/bun/dialect/mysqldialect"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/extra/bundebug"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.TODO()

var allDBs = map[string]func(tb testing.TB) *bun.DB{
	"pg":     pg,
	"pgx":    pgx,
	"mysql8": mysql8,
	"mysql5": mysql5,
	"sqlite": sqlite,
}

func pg(tb testing.TB) *bun.DB {
	dsn := os.Getenv("PG")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/test?sslmode=disable"
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	tb.Cleanup(func() {
		assert.NoError(tb, sqldb.Close())
	})

	db := bun.NewDB(sqldb, pgdialect.New())
	require.Equal(tb, "DB<dialect=pg>", db.String())
	return db
}

func pgx(tb testing.TB) *bun.DB {
	dsn := os.Getenv("PG")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/test?sslmode=disable"
	}

	sqldb, err := sql.Open("pgx", dsn)
	require.NoError(tb, err)
	tb.Cleanup(func() {
		assert.NoError(tb, sqldb.Close())
	})

	db := bun.NewDB(sqldb, pgdialect.New())
	require.Equal(tb, "DB<dialect=pg>", db.String())
	return db
}

func mysql8(tb testing.TB) *bun.DB {
	dsn := os.Getenv("MYSQL")
	if dsn == "" {
		dsn = "user:pass@/test"
	}

	sqldb, err := sql.Open("mysql", dsn)
	require.NoError(tb, err)
	tb.Cleanup(func() {
		assert.NoError(tb, sqldb.Close())
	})

	db := bun.NewDB(sqldb, mysqldialect.New())
	require.Equal(tb, "DB<dialect=mysql8>", db.String())
	return db
}

func mysql5(tb testing.TB) *bun.DB {
	dsn := os.Getenv("MYSQL5")
	if dsn == "" {
		dsn = "user:pass@tcp(localhost:53306)/test"
	}

	sqldb, err := sql.Open("mysql", dsn)
	require.NoError(tb, err)
	tb.Cleanup(func() {
		assert.NoError(tb, sqldb.Close())
	})

	db := bun.NewDB(sqldb, mysqldialect.New())
	require.Equal(tb, "DB<dialect=mysql5>", db.String())
	return db
}

func sqlite(tb testing.TB) *bun.DB {
	sqldb, err := sql.Open(sqliteshim.DriverName(), filepath.Join(tb.TempDir(), "sqlite.db"))
	require.NoError(tb, err)
	tb.Cleanup(func() {
		assert.NoError(tb, sqldb.Close())
	})

	db := bun.NewDB(sqldb, sqlitedialect.New())
	require.Equal(tb, "DB<dialect=sqlite>", db.String())
	return db
}

func testEachDB(t *testing.T, f func(t *testing.T, db *bun.DB)) {
	for name, newDB := range allDBs {
		t.Run(name, func(t *testing.T) {
			db := newDB(t)
			if _, ok := os.LookupEnv("DEBUG"); ok {
				db.AddQueryHook(bundebug.NewQueryHook(bundebug.WithVerbose()))
			}
			f(t, db)
		})
	}
}

func TestDB(t *testing.T) {
	type Test struct {
		name string
		run  func(t *testing.T, db *bun.DB)
	}

	tests := []Test{
		{"testPing", testPing},
		{"testNilModel", testNilModel},
		{"testSelectScan", testSelectScan},
		{"testSelectCount", testSelectCount},
		{"testSelectMap", testSelectMap},
		{"testSelectMapSlice", testSelectMapSlice},
		{"testSelectStruct", testSelectStruct},
		{"testSelectNestedStructValue", testSelectNestedStructValue},
		{"testSelectNestedStructPtr", testSelectNestedStructPtr},
		{"testSelectStructSlice", testSelectStructSlice},
		{"testSelectSingleSlice", testSelectSingleSlice},
		{"testSelectMultiSlice", testSelectMultiSlice},
		{"testSelectJSONMap", testSelectJSONMap},
		{"testSelectJSONStruct", testSelectJSONStruct},
		{"testJSONSpecialChars", testJSONSpecialChars},
		{"testSelectRawMessage", testSelectRawMessage},
		{"testScanNullVar", testScanNullVar},
		{"testScanSingleRow", testScanSingleRow},
		{"testScanSingleRowByRow", testScanSingleRowByRow},
		{"testScanRows", testScanRows},
		{"testRunInTx", testRunInTx},
		{"testInsertIface", testInsertIface},
		{"testSelectBool", testSelectBool},
	}

	testEachDB(t, func(t *testing.T, db *bun.DB) {
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				test.run(t, db)
			})
		}
	})
}

func testPing(t *testing.T, db *bun.DB) {
	err := db.PingContext(ctx)
	require.NoError(t, err)
}

func testNilModel(t *testing.T, db *bun.DB) {
	err := db.NewSelect().ColumnExpr("1").Scan(ctx)
	require.Error(t, err)
	require.Equal(t, "bun: Model(nil)", err.Error())
}

func testSelectScan(t *testing.T, db *bun.DB) {
	var num int
	err := db.NewSelect().ColumnExpr("10").Scan(ctx, &num)
	require.NoError(t, err)
	require.Equal(t, 10, num)

	err = db.NewSelect().TableExpr("(SELECT 10) AS t").Where("FALSE").Scan(ctx, &num)
	require.Equal(t, sql.ErrNoRows, err)
}

func testSelectCount(t *testing.T, db *bun.DB) {
	if db.Dialect().Name() == dialect.MySQL5 {
		t.Skip()
	}

	values := db.NewValues(&[]map[string]interface{}{
		{"num": 1},
		{"num": 2},
		{"num": 3},
	})

	count, err := db.NewSelect().
		With("t", values).
		TableExpr("t").
		OrderExpr("t.num DESC").
		Limit(1).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, count)
}

func testSelectMap(t *testing.T, db *bun.DB) {
	var m map[string]interface{}
	err := db.NewSelect().
		ColumnExpr("10 AS num").
		Scan(ctx, &m)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{
		"num": int64(10),
	}, m)
}

func testSelectMapSlice(t *testing.T, db *bun.DB) {
	if db.Dialect().Name() == dialect.MySQL5 {
		t.Skip()
	}

	values := db.NewValues(&[]map[string]interface{}{
		{"column1": 1},
		{"column1": 2},
		{"column1": 3},
	})

	var ms []map[string]interface{}
	err := db.NewSelect().
		With("t", values).
		TableExpr("t").
		Scan(ctx, &ms)
	require.NoError(t, err)
	require.Len(t, ms, 3)
	for i, m := range ms {
		require.Equal(t, map[string]interface{}{
			"column1": int64(i + 1),
		}, m)
	}
}

func testSelectStruct(t *testing.T, db *bun.DB) {
	type Model struct {
		Num int
		Str string
	}

	model := new(Model)
	err := db.NewSelect().
		ColumnExpr("10 AS num, 'hello' as str").
		Scan(ctx, model)
	require.NoError(t, err)
	require.Equal(t, 10, model.Num)
	require.Equal(t, "hello", model.Str)

	err = db.NewSelect().TableExpr("(SELECT 42) AS t").Where("FALSE").Scan(ctx, model)
	require.Equal(t, sql.ErrNoRows, err)

	err = db.NewSelect().ColumnExpr("1 as unknown_column").Scan(ctx, model)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Model does not have column")
}

func testSelectNestedStructValue(t *testing.T, db *bun.DB) {
	type Model struct {
		Num int
		Sub struct {
			Str string
		}
	}

	model := new(Model)
	err := db.NewSelect().
		ColumnExpr("10 AS num, 'hello' as sub__str").
		Scan(ctx, model)
	require.NoError(t, err)
	require.Equal(t, 10, model.Num)
	require.Equal(t, "hello", model.Sub.Str)
}

func testSelectNestedStructPtr(t *testing.T, db *bun.DB) {
	type Sub struct {
		Str string
	}

	type Model struct {
		Num int
		Sub *Sub
	}

	model := new(Model)
	err := db.NewSelect().
		ColumnExpr("10 AS num").
		Scan(ctx, model)
	require.NoError(t, err)
	require.Equal(t, 10, model.Num)
	require.Nil(t, model.Sub)

	model = new(Model)
	err = db.NewSelect().
		ColumnExpr("10 AS num, 'hello' AS sub__str").
		Scan(ctx, model)
	require.NoError(t, err)
	require.Equal(t, 10, model.Num)
	require.Equal(t, "hello", model.Sub.Str)

	model = new(Model)
	err = db.NewSelect().
		ColumnExpr("10 AS num, NULL AS sub__str").
		Scan(ctx, model)
	require.NoError(t, err)
	require.Equal(t, 10, model.Num)
	require.Nil(t, model.Sub)
}

func testSelectStructSlice(t *testing.T, db *bun.DB) {
	if db.Dialect().Name() == dialect.MySQL5 {
		t.Skip()
	}

	type Model struct {
		Num int `bun:"column1"`
	}

	values := db.NewValues(&[]map[string]interface{}{
		{"column1": 1},
		{"column1": 2},
		{"column1": 3},
	})

	models := make([]Model, 0)
	err := db.NewSelect().
		With("t", values).
		TableExpr("t").
		Scan(ctx, &models)
	require.NoError(t, err)
	require.Len(t, models, 3)
	for i, model := range models {
		require.Equal(t, i+1, model.Num)
	}
}

func testSelectSingleSlice(t *testing.T, db *bun.DB) {
	if db.Dialect().Name() == dialect.MySQL5 {
		t.Skip()
	}

	values := db.NewValues(&[]map[string]interface{}{
		{"column1": 1},
		{"column1": 2},
		{"column1": 3},
	})

	var ns []int
	err := db.NewSelect().
		With("t", values).
		TableExpr("t").
		Scan(ctx, &ns)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3}, ns)
}

func testSelectMultiSlice(t *testing.T, db *bun.DB) {
	if db.Dialect().Name() == dialect.MySQL5 {
		t.Skip()
	}

	values := db.NewValues(&[]map[string]interface{}{
		{"a": 1, "b": "foo"},
		{"a": 2, "b": "bar"},
		{"a": 3, "b": ""},
	})

	var ns []int
	var ss []string
	err := db.NewSelect().
		With("t", values).
		TableExpr("t").
		Scan(ctx, &ns, &ss)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3}, ns)
	require.Equal(t, []string{"foo", "bar", ""}, ss)
}

func testSelectJSONMap(t *testing.T, db *bun.DB) {
	type Model struct {
		Map map[string]string
	}

	model := new(Model)
	err := db.NewSelect().
		ColumnExpr("? AS map", map[string]string{"hello": "world"}).
		Scan(ctx, model)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"hello": "world"}, model.Map)

	err = db.NewSelect().
		ColumnExpr("NULL AS map").
		Scan(ctx, model)
	require.NoError(t, err)
	require.Equal(t, map[string]string(nil), model.Map)
}

func testSelectJSONStruct(t *testing.T, db *bun.DB) {
	type Struct struct {
		Hello string
	}

	type Model struct {
		Struct Struct
	}

	model := new(Model)
	err := db.NewSelect().
		ColumnExpr("? AS struct", Struct{Hello: "world"}).
		Scan(ctx, model)
	require.NoError(t, err)
	require.Equal(t, Struct{Hello: "world"}, model.Struct)

	err = db.NewSelect().
		ColumnExpr("NULL AS struct").
		Scan(ctx, model)
	require.NoError(t, err)
	require.Equal(t, Struct{}, model.Struct)
}

func testSelectRawMessage(t *testing.T, db *bun.DB) {
	type Model struct {
		Raw json.RawMessage
	}

	model := new(Model)
	err := db.NewSelect().
		ColumnExpr("? AS raw", map[string]string{"hello": "world"}).
		Scan(ctx, model)
	require.NoError(t, err)
	require.Equal(t, `{"hello":"world"}`, string(model.Raw))

	err = db.NewSelect().
		ColumnExpr("NULL AS raw").
		Scan(ctx, model)
	require.NoError(t, err)
	require.Nil(t, model.Raw)
}

func testScanNullVar(t *testing.T, db *bun.DB) {
	num := int(42)
	err := db.NewSelect().ColumnExpr("NULL").Scan(ctx, &num)
	require.NoError(t, err)
	require.Zero(t, num)
}

func testScanSingleRow(t *testing.T, db *bun.DB) {
	rows, err := db.QueryContext(ctx, "SELECT 42")
	require.NoError(t, err)
	defer rows.Close()

	if !rows.Next() {
		t.Fail()
	}

	var num int
	err = db.ScanRow(ctx, rows, &num)
	require.NoError(t, err)
	require.Equal(t, 42, num)
}

func testScanSingleRowByRow(t *testing.T, db *bun.DB) {
	if db.Dialect().Name() == dialect.MySQL5 {
		t.Skip()
	}

	values := db.NewValues(&[]map[string]interface{}{
		{"num": 1},
		{"num": 2},
		{"num": 3},
	})

	rows, err := db.NewSelect().
		With("t", values).
		TableExpr("t").
		OrderExpr("t.num DESC").
		Rows(ctx)
	require.NoError(t, err)
	defer rows.Close()

	var nums []int

	for rows.Next() {
		var num int

		err := db.ScanRow(ctx, rows, &num)
		require.NoError(t, err)

		nums = append(nums, num)
	}

	require.NoError(t, rows.Err())
	require.Equal(t, []int{3, 2, 1}, nums)
}

func testScanRows(t *testing.T, db *bun.DB) {
	if db.Dialect().Name() == dialect.MySQL5 {
		t.Skip()
	}

	values := db.NewValues(&[]map[string]interface{}{
		{"num": 1},
		{"num": 2},
		{"num": 3},
	})

	rows, err := db.NewSelect().
		With("t", values).
		TableExpr("t").
		OrderExpr("t.num DESC").
		Rows(ctx)
	require.NoError(t, err)
	defer rows.Close()

	var nums []int
	err = db.ScanRows(ctx, rows, &nums)
	require.NoError(t, err)
	require.Equal(t, []int{3, 2, 1}, nums)
}

func testRunInTx(t *testing.T, db *bun.DB) {
	type Counter struct {
		Count int64
	}

	err := db.ResetModel(ctx, (*Counter)(nil))
	require.NoError(t, err)

	_, err = db.NewInsert().Model(&Counter{Count: 0}).Exec(ctx)
	require.NoError(t, err)

	err = db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewUpdate().Model((*Counter)(nil)).
			Set("count = count + 1").
			Where("TRUE").
			Exec(ctx)
		return err
	})
	require.NoError(t, err)

	var count int
	err = db.NewSelect().Model((*Counter)(nil)).Scan(ctx, &count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	err = db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewUpdate().Model((*Counter)(nil)).
			Set("count = count + 1").
			Where("TRUE").
			Exec(ctx); err != nil {
			return err
		}
		return errors.New("rollback")
	})
	require.Error(t, err)

	err = db.NewSelect().Model((*Counter)(nil)).Scan(ctx, &count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func testJSONSpecialChars(t *testing.T, db *bun.DB) {
	type Model struct {
		ID    int
		Attrs map[string]interface{} `bun:"type:json"`
	}

	ctx := context.Background()

	err := db.ResetModel(ctx, (*Model)(nil))
	require.NoError(t, err)

	model := &Model{
		Attrs: map[string]interface{}{
			"hello": "\000world\nworld\u0000",
		},
	}
	_, err = db.NewInsert().Model(model).Exec(ctx)
	require.NoError(t, err)

	model = new(Model)
	err = db.NewSelect().Model(model).Scan(ctx)
	require.NoError(t, err)
	switch db.Dialect().Name() {
	case dialect.MySQL5, dialect.MySQL8:
		require.Equal(t, map[string]interface{}{
			"hello": "\x00world\nworld\x00",
		}, model.Attrs)
	default:
		require.Equal(t, map[string]interface{}{
			"hello": "\\u0000world\nworld\\u0000",
		}, model.Attrs)
	}
}

func testInsertIface(t *testing.T, db *bun.DB) {
	type Model struct {
		ID    int
		Value interface{} `bun:"type:json"`
	}

	ctx := context.Background()

	err := db.ResetModel(ctx, (*Model)(nil))
	require.NoError(t, err)

	model := new(Model)
	_, err = db.NewInsert().Model(model).Exec(ctx)
	require.NoError(t, err)

	model = &Model{
		Value: "hello",
	}
	_, err = db.NewInsert().Model(model).Exec(ctx)
	require.NoError(t, err)
}

func testSelectBool(t *testing.T, db *bun.DB) {
	var flag bool
	err := db.NewSelect().ColumnExpr("1").Scan(ctx, &flag)
	require.NoError(t, err)
	require.True(t, flag)

	err = db.NewSelect().ColumnExpr("0").Scan(ctx, &flag)
	require.NoError(t, err)
	require.False(t, flag)
}
