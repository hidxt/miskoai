package pdfsafe

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestPDFUnexpectedValueClosers(t *testing.T) {
	for _, input := range []string{"[ >> ]", "<< /x ] >>", "<< /x >> >>"} {
		t.Run(input, func(t *testing.T) {
			b := newBuffer(strings.NewReader(input), 0, newBudget(context.Background()))
			b.allowEOF = true
			mustFault(t, errMalformed, func() { b.readObject() })
			if b.budget.depth != 0 {
				t.Fatal("malformed value depth did not unwind")
			}
		})
	}
}

type finiteEOF struct {
	attempts int
	prefix   bool
}

func (f *finiteEOF) Read(p []byte) (int, error) {
	f.attempts++
	if !f.prefix {
		f.prefix = true
		p[0] = '<'
		return 1, nil
	}
	if f.attempts > 8 {
		panic(errors.New("finite fixture stop"))
	}
	return 0, io.EOF
}
func TestPDFEOFHexAndASCII85Progress(t *testing.T) {
	t.Run("ASCII85_invalid", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("missing invalid ASCII85 progress guard")
			}
		}()
		newAlphaReader(strings.NewReader("v"), newBudget(context.Background())).Read(make([]byte, 8))
	})
	t.Run("hex_EOF", func(t *testing.T) {
		f := &finiteEOF{}
		b := newBuffer(f, 0, newBudget(context.Background()))
		b.allowEOF = true
		defer func() {
			e := recover()
			if e == nil || f.attempts > 2 {
				t.Error("missing finite EOF guard")
			}
		}()
		b.readToken()
	})
	t.Run("token", func(t *testing.T) {
		b := newBuffer(bytes.NewReader([]byte("("+strings.Repeat("x", 65537)+")")), 0, newBudget(context.Background()))
		defer func() {
			if recover() == nil {
				t.Error("missing token growth guard")
			}
		}()
		b.readToken()
	})
}
func TestPDFBudgetCancelNotRecovered(t *testing.T) {
	b := newBuffer(strings.NewReader("bad"), 0, newBudget(context.Background()))
	defer func() {
		if recover() == nil {
			t.Error("interpreter recovered parse failure and continued")
		}
	}()
	readObjectRecover(b)
}

func TestPDFLexerSharedBounds(t *testing.T) {
	t.Run("EOF_attempt_work", func(t *testing.T) {
		b := newBuffer(strings.NewReader(""), 0, newBudget(context.Background()))
		b.eof = true
		b.budget.work = maxWork - 1
		b.readByte()
		mustFault(t, errLimit, func() { b.readByte() })
	})
	t.Run("depth", func(t *testing.T) {
		b := newBuffer(strings.NewReader(strings.Repeat("[", 65)+"null"+strings.Repeat("]", 65)), 0, newBudget(context.Background()))
		b.allowObjptr = false
		b.allowEOF = true
		mustFault(t, errLimit, func() { b.readObject() })
		if b.budget.depth != 0 {
			t.Fatal("depth did not unwind")
		}
	})
	t.Run("node_before_container", func(t *testing.T) {
		b := newBuffer(strings.NewReader("[null]"), 0, newBudget(context.Background()))
		b.allowObjptr = false
		b.budget.nodes = maxNodes - 1
		mustFault(t, errLimit, func() { b.readObject() })
		if b.budget.nodes > maxNodes {
			t.Fatal("node counter overflow")
		}
	})
	t.Run("retain_before_append", func(t *testing.T) {
		b := newBuffer(strings.NewReader("(x)"), 0, newBudget(context.Background()))
		b.budget.retained = maxRetained
		mustFault(t, errLimit, func() { b.readToken() })
		if len(b.tmp) != 0 {
			t.Fatal("token appended before retention")
		}
	})
	t.Run("token_exact", func(t *testing.T) {
		b := newBuffer(strings.NewReader("("+strings.Repeat("x", maxToken)+")"), 0, newBudget(context.Background()))
		s, ok := b.readToken().(string)
		if !ok || len(s) != maxToken {
			t.Fatal("exact token limit rejected")
		}
	})
	t.Run("odd_hex", func(t *testing.T) {
		b := newBuffer(strings.NewReader("<414>"), 0, newBudget(context.Background()))
		if b.readToken() != "A@" {
			t.Fatal("odd hex padding mismatch")
		}
	})
	t.Run("truncated", func(t *testing.T) {
		for _, s := range []string{"[null", "<< /x 1", "(x", "<41"} {
			b := newBuffer(strings.NewReader(s), 0, newBudget(context.Background()))
			b.allowEOF = true
			b.allowObjptr = false
			mustFault(t, errMalformed, func() { b.readObject() })
		}
	})
}
func TestPDFInterpreterLimitsAndFaults(t *testing.T) {
	t.Run("operand", func(t *testing.T) {
		v, _ := streamFixture([]byte(strings.Repeat("1 ", 257)), nil, nil)
		mustFault(t, errLimit, func() { interpret(v, func(*stack, string) {}) })
		if len(v.r.b.streams) != 0 {
			t.Fatal("operand failure leaked stream")
		}
	})
	t.Run("scopes", func(t *testing.T) {
		v, _ := streamFixture([]byte(strings.Repeat("0 dict begin ", 65)), nil, nil)
		mustFault(t, errLimit, func() { interpret(v, func(*stack, string) {}) })
		if len(v.r.b.streams) != 0 {
			t.Fatal("scope failure leaked stream")
		}
	})
	t.Run("callback_cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		v, _ := streamFixture([]byte("1 op 2 op"), nil, nil)
		v.r.b.ctx = ctx
		mustFault(t, context.Canceled, func() { interpret(v, func(s *stack, _ string) { s.pop(); cancel() }) })
		if len(v.r.b.streams) != 0 {
			t.Fatal("canceled interpreter leaked stream")
		}
	})
	t.Run("safe_work_rethrow", func(t *testing.T) {
		b := newBuffer(strings.NewReader("1"), 0, newBudget(context.Background()))
		b.budget.work = maxWork
		mustFault(t, errLimit, func() { readObjectRecover(b) })
	})
	t.Run("safe_cancel_rethrow", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		b := newBuffer(strings.NewReader("1"), 0, newBudget(ctx))
		cancel()
		mustFault(t, context.Canceled, func() { readObjectRecover(b) })
	})
}

func FuzzPDFLexerSeeds(f *testing.F) {
	for _, s := range []string{"<41>", "<4>", "(hello)", "[null]", "<< /x 1 >>", "<", "[", "("} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > maxToken {
			return
		}
		b := newBuffer(strings.NewReader(s), 0, newBudget(context.Background()))
		b.allowEOF = true
		b.allowObjptr = false
		b.allowStream = false
		defer func() {
			if p := recover(); p != nil {
				if _, ok := p.(fault); !ok {
					t.Errorf("unexpected parser panic %T", p)
				}
			}
		}()
		b.readObject()
	})
}
