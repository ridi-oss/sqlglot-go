package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestParseMySQLCreateTablePartitions(t *testing.T) {
	cases := []struct {
		name                 string
		sql                  string
		propertyKind         exp.Kind
		partitionExpressions int
		createExpressions    int
		innerKind            exp.Kind
		hasMaxValue          bool
	}{
		{
			name:                 "range simple column",
			sql:                  "CREATE TABLE t (id INT, created_at DATE) PARTITION BY RANGE (id) (PARTITION p0 VALUES LESS THAN (10), PARTITION p1 VALUES LESS THAN (20), PARTITION p2 VALUES LESS THAN (MAXVALUE))",
			propertyKind:         exp.KindPartitionByRangeProperty,
			partitionExpressions: 1,
			createExpressions:    3,
			innerKind:            exp.KindPartitionRange,
			hasMaxValue:          true,
		},
		{
			name:                 "range one partition",
			sql:                  "CREATE TABLE t (id INT, name VARCHAR(50)) PARTITION BY RANGE (id) (PARTITION p0 VALUES LESS THAN (100))",
			propertyKind:         exp.KindPartitionByRangeProperty,
			partitionExpressions: 1,
			createExpressions:    1,
			innerKind:            exp.KindPartitionRange,
		},
		{
			name:                 "range year expression",
			sql:                  "CREATE TABLE orders (id INT, order_date DATE) PARTITION BY RANGE (YEAR(order_date)) (PARTITION p2023 VALUES LESS THAN (2024), PARTITION p2024 VALUES LESS THAN (2025), PARTITION pmax VALUES LESS THAN (MAXVALUE))",
			propertyKind:         exp.KindPartitionByRangeProperty,
			partitionExpressions: 1,
			createExpressions:    3,
			innerKind:            exp.KindPartitionRange,
			hasMaxValue:          true,
		},
		{
			name:                 "range month expression",
			sql:                  "CREATE TABLE sales (id INT, sale_date DATE) PARTITION BY RANGE (MONTH(sale_date)) (PARTITION q1 VALUES LESS THAN (4), PARTITION q2 VALUES LESS THAN (7), PARTITION q3 VALUES LESS THAN (10), PARTITION q4 VALUES LESS THAN (13))",
			propertyKind:         exp.KindPartitionByRangeProperty,
			partitionExpressions: 1,
			createExpressions:    4,
			innerKind:            exp.KindPartitionRange,
		},
		{
			name:                 "list simple column",
			sql:                  "CREATE TABLE t (id INT, region VARCHAR(10)) PARTITION BY LIST (id) (PARTITION p_east VALUES IN (1, 2, 3), PARTITION p_west VALUES IN (4, 5, 6))",
			propertyKind:         exp.KindPartitionByListProperty,
			partitionExpressions: 1,
			createExpressions:    2,
			innerKind:            exp.KindPartitionList,
		},
		{
			name:                 "list one partition",
			sql:                  "CREATE TABLE t (id INT) PARTITION BY LIST (id) (PARTITION p0 VALUES IN (1, 2))",
			propertyKind:         exp.KindPartitionByListProperty,
			partitionExpressions: 1,
			createExpressions:    1,
			innerKind:            exp.KindPartitionList,
		},
		{
			name:                 "list employees",
			sql:                  "CREATE TABLE employees (id INT, store_id INT) PARTITION BY LIST (store_id) (PARTITION pNorth VALUES IN (3, 5, 6), PARTITION pSouth VALUES IN (1, 2, 10))",
			propertyKind:         exp.KindPartitionByListProperty,
			partitionExpressions: 1,
			createExpressions:    2,
			innerKind:            exp.KindPartitionList,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			create := parseOneDialect(t, tc.sql, "mysql")
			if create.Kind() != exp.KindCreate {
				t.Fatalf("kind = %v, want Create:\n%s", create.Kind(), create.ToS())
			}

			properties := exprArg(t, create, "properties").Expressions()
			if len(properties) != 1 {
				t.Fatalf("property count = %d, want 1:\n%s", len(properties), create.ToS())
			}
			property := properties[0]
			if property.Kind() != tc.propertyKind {
				t.Fatalf("property kind = %v, want %v:\n%s", property.Kind(), tc.propertyKind, create.ToS())
			}

			partitionExpressions := expressionsForArg(property, "partition_expressions")
			if len(partitionExpressions) != tc.partitionExpressions {
				t.Fatalf("partition expression count = %d, want %d:\n%s", len(partitionExpressions), tc.partitionExpressions, create.ToS())
			}
			createExpressions := expressionsForArg(property, "create_expressions")
			if len(createExpressions) != tc.createExpressions {
				t.Fatalf("create expression count = %d, want %d:\n%s", len(createExpressions), tc.createExpressions, create.ToS())
			}

			for i, partition := range createExpressions {
				if partition.Kind() != exp.KindPartition {
					t.Fatalf("create expression %d kind = %v, want Partition:\n%s", i, partition.Kind(), create.ToS())
				}
				inner := partition.Expressions()
				if len(inner) != 1 || inner[0].Kind() != tc.innerKind {
					t.Fatalf("partition %d inner expressions = %#v, want one %v:\n%s", i, inner, tc.innerKind, create.ToS())
				}
			}

			if tc.hasMaxValue {
				lastPartition := createExpressions[len(createExpressions)-1].Expressions()[0]
				values := lastPartition.Expressions()
				if len(values) != 1 || values[0].Kind() != exp.KindVar || values[0].Name() != "MAXVALUE" {
					t.Fatalf("MAXVALUE shape = %#v, want one Var(MAXVALUE):\n%s", values, create.ToS())
				}
			}
		})
	}
}
