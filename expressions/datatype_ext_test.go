package expressions_test

import (
	"testing"

	_ "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestDataTypeBuilder(t *testing.T) {
	cases := []struct {
		dtype   string
		dialect string
		want    string
	}{
		{"TEXT", "", "TEXT"},
		{"DECIMAL(10, 2)", "", "DECIMAL(10, 2)"},
		{"VARCHAR(255)", "", "VARCHAR(255)"},
		{"ARRAY<INT>", "", "ARRAY<INT>"},
		{"CHAR", "", "CHAR"},
		{"NCHAR", "", "CHAR"},
		{"VARCHAR", "", "VARCHAR"},
		{"NVARCHAR", "", "VARCHAR"},
		{"BINARY", "", "BINARY"},
		{"VARBINARY", "", "VARBINARY"},
		{"INT", "", "INT"},
		{"TINYINT", "", "TINYINT"},
		{"SMALLINT", "", "SMALLINT"},
		{"BIGINT", "", "BIGINT"},
		{"FLOAT", "", "FLOAT"},
		{"DOUBLE", "", "DOUBLE"},
		{"DECIMAL", "", "DECIMAL"},
		{"BOOLEAN", "", "BOOLEAN"},
		{"JSON", "", "JSON"},
		{"JSONB", "postgres", "JSONB"},
		{"INTERVAL", "", "INTERVAL"},
		{"TIME", "", "TIME"},
		{"TIMESTAMP", "", "TIMESTAMP"},
		{"TIMESTAMPTZ", "", "TIMESTAMPTZ"},
		{"TIMESTAMPLTZ", "", "TIMESTAMPLTZ"},
		{"DATE", "", "DATE"},
		{"DATETIME", "", "DATETIME"},
		{"ARRAY", "", "ARRAY"},
		{"MAP", "", "MAP"},
		{"UUID", "", "UUID"},
		{"GEOGRAPHY", "", "GEOGRAPHY"},
		{"GEOMETRY", "", "GEOMETRY"},
		{"STRUCT", "", "STRUCT"},
		{"HSTORE", "postgres", "HSTORE"},
		{"NULL", "", "NULL"},
		{"UNKNOWN", "", "UNKNOWN"},
		{"USER-DEFINED", "", "USER-DEFINED"},
		{"ARRAY<UNKNOWN>", "", "ARRAY<UNKNOWN>"},
		{"ARRAY<NULL>", "", "ARRAY<NULL>"},
		{"varchar(100) collate 'en-ci'", "", "VARCHAR(100)"},
	}
	for _, tc := range cases {
		t.Run(tc.dtype, func(t *testing.T) {
			e, err := exp.DataTypeBuild(tc.dtype, tc.dialect, false, true, nil)
			if err != nil {
				t.Fatalf("DataTypeBuild(%q, %q) error: %v", tc.dtype, tc.dialect, err)
			}
			got, err := e.SQL(exp.GenerateOptions{})
			if err != nil {
				t.Fatalf("SQL(%q) error: %v", tc.dtype, err)
			}
			if got != tc.want {
				t.Fatalf("DataTypeBuild(%q).SQL() = %q, want %q", tc.dtype, got, tc.want)
			}
		})
	}

	if _, err := exp.DataTypeBuild("varchar(", "", false, true, nil); err == nil {
		t.Fatal("DataTypeBuild(varchar() should return a parse error")
	}

	// TODO(slice 5): port duckdb/spark/redshift/bigquery/snowflake dialect-specific cases.
}

func TestIsType(t *testing.T) {
	to := parseOne(t, "CAST(x AS VARCHAR)").Arg("to").(exp.Expression)
	if !exp.DataTypeIsType(to, false, "VARCHAR") {
		t.Fatal("VARCHAR should match VARCHAR")
	}
	if exp.DataTypeIsType(to, false, "VARCHAR(5)") {
		t.Fatal("VARCHAR should not structurally match VARCHAR(5)")
	}
	if exp.DataTypeIsType(to, false, "FLOAT") {
		t.Fatal("VARCHAR should not match FLOAT")
	}

	to = parseOne(t, "CAST(x AS VARCHAR(5))").Arg("to").(exp.Expression)
	if !exp.DataTypeIsType(to, false, "VARCHAR") {
		t.Fatal("VARCHAR(5) should match VARCHAR by type")
	}
	if !exp.DataTypeIsType(to, false, "VARCHAR(5)") {
		t.Fatal("VARCHAR(5) should structurally match VARCHAR(5)")
	}
	if exp.DataTypeIsType(to, false, "VARCHAR(4)") {
		t.Fatal("VARCHAR(5) should not structurally match VARCHAR(4)")
	}
	if exp.DataTypeIsType(to, false, "FLOAT") {
		t.Fatal("VARCHAR(5) should not match FLOAT")
	}

	to = parseOne(t, "CAST(x AS ARRAY<INT>)").Arg("to").(exp.Expression)
	if !exp.DataTypeIsType(to, false, "ARRAY") {
		t.Fatal("ARRAY<INT> should match ARRAY by type")
	}
	if !exp.DataTypeIsType(to, false, "ARRAY<INT>") {
		t.Fatal("ARRAY<INT> should structurally match ARRAY<INT>")
	}
	if exp.DataTypeIsType(to, false, "ARRAY<FLOAT>") {
		t.Fatal("ARRAY<INT> should not structurally match ARRAY<FLOAT>")
	}
	if exp.DataTypeIsType(to, false, "INT") {
		t.Fatal("ARRAY<INT> should not match INT")
	}

	to = parseOne(t, "CAST(x AS ARRAY)").Arg("to").(exp.Expression)
	if !exp.DataTypeIsType(to, false, "ARRAY") {
		t.Fatal("ARRAY should match ARRAY")
	}
	if exp.DataTypeIsType(to, false, "ARRAY<INT>") {
		t.Fatal("ARRAY should not structurally match ARRAY<INT>")
	}
	if exp.DataTypeIsType(to, false, "ARRAY<FLOAT>") {
		t.Fatal("ARRAY should not structurally match ARRAY<FLOAT>")
	}
	if exp.DataTypeIsType(to, false, "INT") {
		t.Fatal("ARRAY should not match INT")
	}

	to = parseOne(t, "CAST(x AS STRUCT<a INT, b FLOAT>)").Arg("to").(exp.Expression)
	if !exp.DataTypeIsType(to, false, "STRUCT") {
		t.Fatal("STRUCT<a INT, b FLOAT> should match STRUCT by type")
	}
	if !exp.DataTypeIsType(to, false, "STRUCT<a INT, b FLOAT>") {
		t.Fatal("STRUCT<a INT, b FLOAT> should structurally match itself")
	}
	if exp.DataTypeIsType(to, false, "STRUCT<a VARCHAR, b INT>") {
		t.Fatal("STRUCT<a INT, b FLOAT> should not structurally match a different struct")
	}

	dtype, err := exp.DataTypeBuild("foo", "", true, true, nil)
	if err != nil {
		t.Fatalf("DataTypeBuild(foo, udt=true) error: %v", err)
	}
	if !exp.DataTypeIsType(dtype, false, "foo") {
		t.Fatal("USERDEFINED foo should match foo")
	}
	if exp.DataTypeIsType(dtype, false, "bar") {
		t.Fatal("USERDEFINED foo should not match bar")
	}

	dtype, err = exp.DataTypeBuild("a.b.c", "", true, true, nil)
	if err != nil {
		t.Fatalf("DataTypeBuild(a.b.c, udt=true) error: %v", err)
	}
	if !exp.DataTypeIsType(dtype, false, "a.b.c") {
		t.Fatal("USERDEFINED a.b.c should match a.b.c")
	}

	if _, err := exp.DataTypeBuild("foo", "", false, true, nil); err == nil {
		t.Fatal("DataTypeBuild(foo, udt=false) should return a parse error")
	}

	// TODO(slice 5): port Nullable(Int32)/check_nullable clickhouse cases.
}

func TestEqDataType(t *testing.T) {
	dataType, err := exp.DataTypeBuild("int", "", false, true, nil)
	if err != nil {
		t.Fatalf("DataTypeBuild(int) error: %v", err)
	}
	if !dataType.Equal(exp.DataType(exp.Args{"this": exp.DTypeInt, "nested": false})) {
		t.Fatalf("DataTypeBuild(int) did not equal INT DataType:\n%s", dataType.ToS())
	}
}
