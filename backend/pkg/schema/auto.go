package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// AutoKeyword marks a top-level property the server fills: "number" (the next
// whole number of the collection), "created_by" (who made the record) or
// "updated_by" (who changed it last). FormulaKeyword holds arithmetic over
// other number fields ("price * qty"), computed on every write.
const (
	AutoKeyword    = "x-auto"
	FormulaKeyword = "x-formula"

	AutoNumber    = "number"
	AutoCreatedBy = "created_by"
	AutoUpdatedBy = "updated_by"
)

// AutoField is a property the server fills.
type AutoField struct {
	Key  string
	Kind string // AutoNumber, AutoCreatedBy or AutoUpdatedBy
}

func topProps(raw []byte) map[string]map[string]any {
	var doc struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	return doc.Properties
}

// AutoKeys lists the filled properties, sorted by key.
func AutoKeys(raw []byte) []AutoField {
	var out []AutoField
	for k, p := range topProps(raw) {
		if kind, _ := p[AutoKeyword].(string); kind == AutoNumber || kind == AutoCreatedBy || kind == AutoUpdatedBy {
			out = append(out, AutoField{Key: k, Kind: kind})
		}
	}
	slices.SortFunc(out, func(a, b AutoField) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// Formulas maps each formula property to its expression.
func Formulas(raw []byte) map[string]string {
	out := map[string]string{}
	for k, p := range topProps(raw) {
		if f, _ := p[FormulaKeyword].(string); f != "" {
			out[k] = f
		}
	}
	return out
}

// Formula is a parsed expression: numbers, field keys, + - * / and parentheses.
type Formula struct{ root node }

type node interface {
	eval(vals map[string]json.RawMessage) (float64, bool)
}

type num float64
type field string
type neg struct{ x node }
type bin struct {
	op   byte
	l, r node
}

func (n num) eval(map[string]json.RawMessage) (float64, bool) { return float64(n), true }
func (f field) eval(vals map[string]json.RawMessage) (float64, bool) {
	var v float64
	if json.Unmarshal(vals[string(f)], &v) != nil {
		return 0, false // missing or not a number: the formula has no value
	}
	return v, true
}
func (n neg) eval(vals map[string]json.RawMessage) (float64, bool) {
	v, ok := n.x.eval(vals)
	return -v, ok
}
func (b bin) eval(vals map[string]json.RawMessage) (float64, bool) {
	l, ok1 := b.l.eval(vals)
	r, ok2 := b.r.eval(vals)
	if !ok1 || !ok2 {
		return 0, false
	}
	switch b.op {
	case '+':
		return l + r, true
	case '-':
		return l - r, true
	case '*':
		return l * r, true
	}
	if r == 0 {
		return 0, false
	}
	return l / r, true
}

// Eval computes the formula; ok is false when a field it uses is missing or
// not a number, or it divides by zero.
func (f *Formula) Eval(vals map[string]json.RawMessage) (float64, bool) {
	v, ok := f.root.eval(vals)
	return v, ok && !math.IsInf(v, 0) && !math.IsNaN(v)
}

// Fields lists the keys the formula reads.
func (f *Formula) Fields() []string {
	var out []string
	var walk func(n node)
	walk = func(n node) {
		switch x := n.(type) {
		case field:
			out = append(out, string(x))
		case neg:
			walk(x.x)
		case bin:
			walk(x.l)
			walk(x.r)
		}
	}
	walk(f.root)
	return out
}

type parser struct {
	src string
	pos int
}

// ParseFormula reads an expression of up to 500 characters.
func ParseFormula(src string) (*Formula, error) {
	if len(src) > 500 {
		return nil, errors.New("formula longer than 500 characters")
	}
	p := &parser{src: src}
	n, err := p.expr(0)
	if err != nil {
		return nil, err
	}
	if p.skip(); p.pos < len(p.src) {
		return nil, fmt.Errorf("unexpected %q at %d", p.src[p.pos], p.pos+1)
	}
	return &Formula{root: n}, nil
}

func (p *parser) skip() {
	for p.pos < len(p.src) && p.src[p.pos] == ' ' {
		p.pos++
	}
}

func (p *parser) expr(depth int) (node, error) {
	if depth > 50 {
		return nil, errors.New("formula nests too deep")
	}
	l, err := p.term(depth)
	for err == nil {
		p.skip()
		if p.pos >= len(p.src) || (p.src[p.pos] != '+' && p.src[p.pos] != '-') {
			return l, nil
		}
		op := p.src[p.pos]
		p.pos++
		var r node
		if r, err = p.term(depth); err == nil {
			l = bin{op: op, l: l, r: r}
		}
	}
	return nil, err
}

func (p *parser) term(depth int) (node, error) {
	l, err := p.factor(depth)
	for err == nil {
		p.skip()
		if p.pos >= len(p.src) || (p.src[p.pos] != '*' && p.src[p.pos] != '/') {
			return l, nil
		}
		op := p.src[p.pos]
		p.pos++
		var r node
		if r, err = p.factor(depth); err == nil {
			l = bin{op: op, l: l, r: r}
		}
	}
	return nil, err
}

func (p *parser) factor(depth int) (node, error) {
	p.skip()
	if p.pos >= len(p.src) {
		return nil, errors.New("formula ends too soon")
	}
	c := p.src[p.pos]
	switch {
	case c == '(':
		p.pos++
		n, err := p.expr(depth + 1)
		if err != nil {
			return nil, err
		}
		if p.skip(); p.pos >= len(p.src) || p.src[p.pos] != ')' {
			return nil, errors.New("missing )")
		}
		p.pos++
		return n, nil
	case c == '-':
		p.pos++
		n, err := p.factor(depth + 1)
		return neg{n}, err
	case c >= '0' && c <= '9' || c == '.':
		start := p.pos
		for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9' || p.src[p.pos] == '.') {
			p.pos++
		}
		v, err := strconv.ParseFloat(p.src[start:p.pos], 64)
		if err != nil {
			return nil, fmt.Errorf("bad number %q", p.src[start:p.pos])
		}
		return num(v), nil
	case c == '_' || unicode.IsLetter(rune(c)):
		start := p.pos
		for p.pos < len(p.src) && (p.src[p.pos] == '_' || unicode.IsLetter(rune(p.src[p.pos])) || unicode.IsDigit(rune(p.src[p.pos]))) {
			p.pos++
		}
		return field(p.src[start:p.pos]), nil
	}
	return nil, fmt.Errorf("unexpected %q at %d", c, p.pos+1)
}
