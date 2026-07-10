package tokens

import (
	"errors"
	"reflect"
	"testing"
)

func TestTokenizerCompiledConfigPathRetainsState(t *testing.T) {
	tokenizer := NewTokenizerWithConfig(BaseConfig())
	got, err := tokenizer.Tokenize("SELECT café")
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if tokenizer.SQL() != "SELECT café" {
		t.Fatalf("SQL() = %q, want %q", tokenizer.SQL(), "SELECT café")
	}
	if tokenizer.Size() != 11 {
		t.Fatalf("Size() = %d, want 11 runes", tokenizer.Size())
	}
	if !reflect.DeepEqual(tokenizer.Tokens(), got) {
		t.Fatalf("Tokens() = %s, want returned tokens %s", ReprTokens(tokenizer.Tokens()), ReprTokens(got))
	}
}

func TestNewTokenizerWithFuncRejectsNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewTokenizerWithFunc accepted a nil callback")
		}
	}()
	NewTokenizerWithFunc(nil)
}

func TestTokenizerCustomPathRetainsState(t *testing.T) {
	wantErr := errors.New("custom tokenization failed")
	calls := 0
	tokenizer := NewTokenizerWithFunc(func(sql string) ([]Token, error) {
		calls++
		if sql == "bad café" {
			return []Token{NewToken(VAR, "partial")}, wantErr
		}
		return []Token{NewToken(VAR, sql)}, nil
	})

	got, err := tokenizer.Tokenize("café")
	if err != nil {
		t.Fatalf("Tokenize(custom): %v", err)
	}
	if calls != 1 {
		t.Fatalf("custom callback calls = %d, want 1", calls)
	}
	if tokenizer.SQL() != "café" || tokenizer.Size() != 4 {
		t.Fatalf("custom state SQL=%q Size=%d, want café/4", tokenizer.SQL(), tokenizer.Size())
	}
	if !reflect.DeepEqual(tokenizer.Tokens(), got) {
		t.Fatalf("Tokens() = %s, want returned tokens %s", ReprTokens(tokenizer.Tokens()), ReprTokens(got))
	}

	got, err = tokenizer.Tokenize("bad café")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Tokenize(custom error) = %v, want %v", err, wantErr)
	}
	if calls != 2 {
		t.Fatalf("custom callback calls = %d, want 2", calls)
	}
	if tokenizer.SQL() != "bad café" || tokenizer.Size() != 8 {
		t.Fatalf("custom error state SQL=%q Size=%d, want bad café/8", tokenizer.SQL(), tokenizer.Size())
	}
	if !reflect.DeepEqual(tokenizer.Tokens(), got) || len(got) != 1 || got[0].Text != "partial" {
		t.Fatalf("custom partial Tokens() = %s, returned %s", ReprTokens(tokenizer.Tokens()), ReprTokens(got))
	}
}
