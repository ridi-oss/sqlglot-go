package generator_test

import (
	"testing"

	"github.com/ridi/sqlglot-go/dialects"
	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestMySQLCreateTablePartitionsSQL(t *testing.T) {
	cases := []string{
		"CREATE TABLE t (id INT, created_at DATE) PARTITION BY RANGE (id) (PARTITION p0 VALUES LESS THAN (10), PARTITION p1 VALUES LESS THAN (20), PARTITION p2 VALUES LESS THAN (MAXVALUE))",
		"CREATE TABLE t (id INT, name VARCHAR(50)) PARTITION BY RANGE (id) (PARTITION p0 VALUES LESS THAN (100))",
		"CREATE TABLE orders (id INT, order_date DATE) PARTITION BY RANGE (YEAR(order_date)) (PARTITION p2023 VALUES LESS THAN (2024), PARTITION p2024 VALUES LESS THAN (2025), PARTITION pmax VALUES LESS THAN (MAXVALUE))",
		"CREATE TABLE sales (id INT, sale_date DATE) PARTITION BY RANGE (MONTH(sale_date)) (PARTITION q1 VALUES LESS THAN (4), PARTITION q2 VALUES LESS THAN (7), PARTITION q3 VALUES LESS THAN (10), PARTITION q4 VALUES LESS THAN (13))",
		"CREATE TABLE t (id INT, region VARCHAR(10)) PARTITION BY LIST (id) (PARTITION p_east VALUES IN (1, 2, 3), PARTITION p_west VALUES IN (4, 5, 6))",
		"CREATE TABLE t (id INT) PARTITION BY LIST (id) (PARTITION p0 VALUES IN (1, 2))",
		"CREATE TABLE employees (id INT, store_id INT) PARTITION BY LIST (store_id) (PARTITION pNorth VALUES IN (3, 5, 6), PARTITION pSouth VALUES IN (1, 2, 10))",
	}

	for _, sql := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := roundTrip(t, "mysql", sql); got != sql {
				t.Fatalf("round-trip mismatch:\n  got  %q\n  want %q", got, sql)
			}
		})
	}
}

func TestMySQLPartitionSQLPreservesStandaloneWrapper(t *testing.T) {
	partition := exp.Partition(exp.Args{
		"expressions": []exp.Expression{exp.Column(exp.Args{"this": exp.ToIdentifier("p")})},
	})
	if got := genSQL(t, dialects.MySQL(), partition); got != "PARTITION(p)" {
		t.Fatalf("standalone Partition = %q, want %q", got, "PARTITION(p)")
	}

	subpartition := exp.Partition(exp.Args{
		"expressions":  []exp.Expression{exp.Column(exp.Args{"this": exp.ToIdentifier("sp")})},
		"subpartition": true,
	})
	if got := genSQL(t, dialects.MySQL(), subpartition); got != "SUBPARTITION(sp)" {
		t.Fatalf("standalone subpartition = %q, want %q", got, "SUBPARTITION(sp)")
	}
}
