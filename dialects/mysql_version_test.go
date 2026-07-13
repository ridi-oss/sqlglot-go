package dialects_test

import (
	"strings"
	"testing"

	"github.com/ridi/sqlglot-go/dialects"
)

func TestMySQLVersionSetting(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		want    int
	}{
		// mysql_version is the MYSQL_VERSION_ID integer verbatim (the /*!NNNNN gate form
		// and what mysql_get_server_version() returns) — not a major version, not dotted.
		{name: "version id 8.0.35", dialect: "mysql, mysql_version=80035", want: 80035},
		{name: "version id 8.0.33", dialect: "mysql, mysql_version=80033", want: 80033},
		{name: "version id 8.4.0", dialect: "mysql, mysql_version=80400", want: 80400},
		{name: "version id 5.7.44", dialect: "mysql, mysql_version=50744", want: 50744},
		{name: "version id 5.7.0", dialect: "mysql, mysql_version=50700", want: 50700},
		{name: "whitespace", dialect: " mysql , mysql_version = 80035 ", want: 80035},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := dialects.GetOrRaise(tt.dialect)
			if err != nil {
				t.Fatalf("GetOrRaise(%q): %v", tt.dialect, err)
			}
			if d.MySQLVersion == nil {
				t.Fatalf("MySQLVersion = nil, want %d", tt.want)
			}
			if got := *d.MySQLVersion; got != tt.want {
				t.Fatalf("MySQLVersion = %d, want %d", got, tt.want)
			}
			if !d.TokenizerConfig.MySQLExecutableComments {
				t.Fatal("MySQLExecutableComments = false, want true")
			}
		})
	}

	bare, err := dialects.GetOrRaise("mysql")
	if err != nil {
		t.Fatalf("GetOrRaise(mysql): %v", err)
	}
	if bare.MySQLVersion != nil {
		t.Fatalf("bare MySQLVersion = %d, want nil", *bare.MySQLVersion)
	}
}

func TestMySQLVersionSettingErrors(t *testing.T) {
	tests := []struct {
		name      string
		dialect   string
		wantError string
	}{
		{name: "missing value delimiter", dialect: "mysql, mysql_version", wantError: `dialect setting "mysql_version" requires a value`},
		{name: "empty", dialect: "mysql, mysql_version=", wantError: "mysql_version"},
		{name: "non numeric start", dialect: "mysql, mysql_version=release-8.0.33", wantError: "mysql_version"},
		{name: "dotted not supported", dialect: "mysql, mysql_version=8.0.33", wantError: "mysql_version"},
		{name: "dotted major minor", dialect: "mysql, mysql_version=8.4", wantError: "mysql_version"},
		{name: "version id with suffix", dialect: "mysql, mysql_version=80035-log", wantError: "mysql_version"},
		{name: "trailing dot", dialect: "mysql, mysql_version=8.", wantError: "mysql_version"},
		{name: "overflow", dialect: "mysql, mysql_version=" + strings.Repeat("9", 100), wantError: "mysql_version"},
		{name: "unknown setting", dialect: "mysql, future_setting=1", wantError: `unsupported dialect setting "future_setting" (supported: normalization_strategy, mysql_version)`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dialects.GetOrRaise(tt.dialect)
			if err == nil {
				t.Fatalf("GetOrRaise(%q) succeeded, want error", tt.dialect)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("GetOrRaise(%q) error = %q, want substring %q", tt.dialect, err, tt.wantError)
			}
		})
	}
}

func TestMySQLVersionSettingAcceptedButInertForOtherDialects(t *testing.T) {
	for _, dialect := range []string{
		", mysql_version=80033",
		"postgres, mysql_version=80033",
	} {
		d, err := dialects.GetOrRaise(dialect)
		if err != nil {
			t.Fatalf("GetOrRaise(%q): %v", dialect, err)
		}
		if d.MySQLVersion == nil || *d.MySQLVersion != 80033 {
			t.Fatalf("GetOrRaise(%q) MySQLVersion = %v, want 80033", dialect, d.MySQLVersion)
		}
		if d.TokenizerConfig.MySQLExecutableComments {
			t.Fatalf("GetOrRaise(%q) MySQLExecutableComments = true, want false", dialect)
		}
	}

	for _, dialect := range []string{
		", mysql_version=8.0",
		"postgres, mysql_version=8.0",
	} {
		if _, err := dialects.GetOrRaise(dialect); err == nil {
			t.Fatalf("GetOrRaise(%q) succeeded, want malformed mysql_version error", dialect)
		} else if !strings.Contains(err.Error(), "mysql_version") {
			t.Fatalf("GetOrRaise(%q) error = %q, want mysql_version", dialect, err)
		}
	}
}
