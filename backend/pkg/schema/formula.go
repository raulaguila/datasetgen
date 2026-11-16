package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Formula is a parsed expression over a record's top-level fields:
//
//	numbers, "text", TRUE, FALSE and field keys
//	+ - * / on numbers, & joins text, = <> < <= > >= compare
//	IF(cond, then, else)  AND(...)  OR(...)  NOT(x)  COALESCE(...)
//	ROUND(x[, digits])  ABS(x)  MIN(...)  MAX(...)
//	CONCAT(...)  UPPER(s)  LOWER(s)  TRIM(s)  LEN(s)
//	TODAY()  DAYS(end, start)  DATEADD(date, days)  YEAR(d)  MONTH(d)  DAY(d)
//
// A missing field, text in arithmetic or a division by zero gives no value,
// and so does everything built on it, unless IF or COALESCE steps around it.
type Formula struct{ root node }

type node interface {
	eval(e *env) any // nil: no value
}

type env struct {
	vals  map[string]json.RawMessage
	today time.Time
}

type lit struct{ v any }
type field string
type neg struct{ x node }
type bin struct {
	op   string
	l, r node
}
type call struct {
	fn   string
	args []node
}

func (n lit) eval(*env) any { return n.v }

func (f field) eval(e *env) any {
	raw, ok := e.vals[string(f)]
	if !ok {
		return nil
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	switch v.(type) {
	case float64, string, bool:
		return v
	}
	return nil // lists and objects have no formula value
}

func (n neg) eval(e *env) any {
	if v, ok := n.x.eval(e).(float64); ok {
		return -v
	}
	return nil
}

func (b bin) eval(e *env) any {
	l, r := b.l.eval(e), b.r.eval(e)
	if l == nil || r == nil {
		return nil
	}
	switch b.op {
	case "&":
		return text(l) + text(r)
	case "=", "<>", "<", "<=", ">", ">=":
		c, ok := compare(l, r)
		if !ok {
			return b.op == "<>" // text never equals a number
		}
		switch b.op {
		case "=":
			return c == 0
		case "<>":
			return c != 0
		case "<":
			return c < 0
		case "<=":
			return c <= 0
		case ">":
			return c > 0
		}
		return c >= 0
	}
	x, ok1 := l.(float64)
	y, ok2 := r.(float64)
	if !ok1 || !ok2 {
		return nil
	}
	switch b.op {
	case "+":
		return x + y
	case "-":
		return x - y
	case "*":
		return x * y
	}
	if y == 0 {
		return nil
	}
	return x / y
}

// compare orders two numbers, two texts (dates as text order by day) or two booleans.
func compare(l, r any) (int, bool) {
	switch x := l.(type) {
	case float64:
		if y, ok := r.(float64); ok {
			return cmpOf(x < y, x > y), true
		}
	case string:
		if y, ok := r.(string); ok {
			return strings.Compare(x, y), true
		}
	case bool:
		if y, ok := r.(bool); ok {
			return cmpOf(!x && y, x && !y), true
		}
	}
	return 0, false
}

func cmpOf(less, more bool) int {
	switch {
	case less:
		return -1
	case more:
		return 1
	}
	return 0
}

func text(v any) string {
	switch x := v.(type) {
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	case string:
		return x
	}
	return ""
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		return x != ""
	}
	return false
}

// day reads a date ("2026-11-16") or a date-time; ok is false for anything else.
func day(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Truncate(24 * time.Hour), true
	}
	return time.Time{}, false
}

// arity is each function's fewest and most arguments; -1 is no limit.
var arity = map[string][2]int{
	"IF": {3, 3}, "AND": {1, -1}, "OR": {1, -1}, "NOT": {1, 1}, "COALESCE": {1, -1},
	"ROUND": {1, 2}, "ABS": {1, 1}, "MIN": {1, -1}, "MAX": {1, -1},
	"CONCAT": {1, -1}, "UPPER": {1, 1}, "LOWER": {1, 1}, "TRIM": {1, 1}, "LEN": {1, 1},
	"TODAY": {0, 0}, "DAYS": {2, 2}, "DATEADD": {2, 2}, "YEAR": {1, 1}, "MONTH": {1, 1}, "DAY": {1, 1},
}

func (c call) eval(e *env) any {
	arg := func(i int) any { return c.args[i].eval(e) }
	num := func(i int) (float64, bool) { v, ok := arg(i).(float64); return v, ok }
	switch c.fn {
	case "IF":
		if truthy(arg(0)) {
			return arg(1)
		}
		return arg(2)
	case "AND", "OR":
		for i := range c.args {
			if t := truthy(arg(i)); t == (c.fn == "OR") {
				return t
			}
		}
		return c.fn == "AND"
	case "NOT":
		return !truthy(arg(0))
	case "COALESCE":
		for i := range c.args {
			if v := arg(i); v != nil && v != "" {
				return v
			}
		}
		return nil
	case "ROUND":
		x, ok := num(0)
		digits := 0.0
		if len(c.args) == 2 {
			var ok2 bool
			if digits, ok2 = num(1); !ok2 {
				return nil
			}
		}
		if !ok || digits < 0 || digits > 10 {
			return nil
		}
		p := math.Pow(10, math.Trunc(digits))
		return math.Round(x*p) / p
	case "ABS":
		if x, ok := num(0); ok {
			return math.Abs(x)
		}
		return nil
	case "MIN", "MAX":
		var out any
		for i := range c.args {
			x, ok := num(i)
			if !ok {
				return nil
			}
			if out == nil || (c.fn == "MIN" && x < out.(float64)) || (c.fn == "MAX" && x > out.(float64)) {
				out = x
			}
		}
		return out
	case "CONCAT":
		var b strings.Builder
		for i := range c.args {
			b.WriteString(text(arg(i)))
		}
		return b.String()
	case "UPPER", "LOWER", "TRIM", "LEN":
		v := arg(0)
		if v == nil {
			return nil
		}
		s := text(v)
		switch c.fn {
		case "UPPER":
			return strings.ToUpper(s)
		case "LOWER":
			return strings.ToLower(s)
		case "TRIM":
			return strings.TrimSpace(s)
		}
		return float64(utf8.RuneCountInString(s))
	case "TODAY":
		return e.today.Format(time.DateOnly)
	case "DAYS":
		end, ok1 := day(arg(0))
		start, ok2 := day(arg(1))
		if !ok1 || !ok2 {
			return nil
		}
		return math.Round(end.Sub(start).Hours() / 24)
	case "DATEADD":
		d, ok1 := day(arg(0))
		n, ok2 := num(1)
		if !ok1 || !ok2 {
			return nil
		}
		return d.AddDate(0, 0, int(n)).Format(time.DateOnly)
	case "YEAR", "MONTH", "DAY":
		d, ok := day(arg(0))
		if !ok {
			return nil
		}
		switch c.fn {
		case "YEAR":
			return float64(d.Year())
		case "MONTH":
			return float64(d.Month())
		}
		return float64(d.Day())
	}
	return nil
}

// Eval computes the formula; ok is false when it has no value. TODAY() answers
// today's date in today's location (formulas run when a record is saved).
func (f *Formula) Eval(vals map[string]json.RawMessage, today time.Time) (any, bool) {
	v := f.root.eval(&env{vals: vals, today: today})
	if x, ok := v.(float64); ok && (math.IsInf(x, 0) || math.IsNaN(x)) {
		return nil, false
	}
	return v, v != nil
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
		case call:
			for _, a := range x.args {
				walk(a)
			}
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
	n, err := p.compare(0)
	if err != nil {
		return nil, err
	}
	if p.skip(); p.pos < len(p.src) {
		return nil, fmt.Errorf("unexpected %q at %d", p.src[p.pos], p.pos+1)
	}
	return &Formula{root: n}, nil
}

func (p *parser) skip() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n') {
		p.pos++
	}
}

// take consumes the first of ops found next, longest first.
func (p *parser) take(ops ...string) string {
	p.skip()
	for _, op := range ops {
		if strings.HasPrefix(p.src[p.pos:], op) {
			p.pos += len(op)
			return op
		}
	}
	return ""
}

func (p *parser) compare(depth int) (node, error) {
	if depth > 50 {
		return nil, errors.New("formula nests too deep")
	}
	l, err := p.join(depth)
	if err != nil {
		return nil, err
	}
	op := p.take("<>", "!=", "<=", ">=", "=", "<", ">")
	if op == "" {
		return l, nil
	}
	if op == "!=" {
		op = "<>"
	}
	r, err := p.join(depth)
	if err != nil {
		return nil, err
	}
	return bin{op: op, l: l, r: r}, nil
}

func (p *parser) join(depth int) (node, error) {
	l, err := p.sum(depth)
	for err == nil {
		if p.take("&") == "" {
			return l, nil
		}
		var r node
		if r, err = p.sum(depth); err == nil {
			l = bin{op: "&", l: l, r: r}
		}
	}
	return nil, err
}

func (p *parser) sum(depth int) (node, error) {
	l, err := p.term(depth)
	for err == nil {
		op := p.take("+", "-")
		if op == "" {
			return l, nil
		}
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
		op := p.take("*", "/")
		if op == "" {
			return l, nil
		}
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
		n, err := p.compare(depth + 1)
		if err != nil {
			return nil, err
		}
		if p.take(")") == "" {
			return nil, errors.New("missing )")
		}
		return n, nil
	case c == '-':
		p.pos++
		n, err := p.factor(depth + 1)
		return neg{n}, err
	case c == '"':
		end := strings.IndexByte(p.src[p.pos+1:], '"')
		if end < 0 {
			return nil, errors.New("text without its closing \"")
		}
		s := p.src[p.pos+1 : p.pos+1+end]
		p.pos += end + 2
		return lit{s}, nil
	case c >= '0' && c <= '9' || c == '.':
		start := p.pos
		for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9' || p.src[p.pos] == '.') {
			p.pos++
		}
		v, err := strconv.ParseFloat(p.src[start:p.pos], 64)
		if err != nil {
			return nil, fmt.Errorf("bad number %q", p.src[start:p.pos])
		}
		return lit{v}, nil
	case c == '_' || unicode.IsLetter(rune(c)):
		start := p.pos
		for p.pos < len(p.src) && (p.src[p.pos] == '_' || unicode.IsLetter(rune(p.src[p.pos])) || unicode.IsDigit(rune(p.src[p.pos]))) {
			p.pos++
		}
		name := p.src[start:p.pos]
		if p.skip(); p.pos < len(p.src) && p.src[p.pos] == '(' {
			return p.call(strings.ToUpper(name), depth)
		}
		switch strings.ToUpper(name) {
		case "TRUE":
			return lit{true}, nil
		case "FALSE":
			return lit{false}, nil
		}
		return field(name), nil
	}
	return nil, fmt.Errorf("unexpected %q at %d", c, p.pos+1)
}

func (p *parser) call(fn string, depth int) (node, error) {
	limits, ok := arity[fn]
	if !ok {
		return nil, fmt.Errorf("unknown function %s", fn)
	}
	p.pos++ // (
	var args []node
	if p.take(")") == "" {
		for {
			a, err := p.compare(depth + 1)
			if err != nil {
				return nil, err
			}
			args = append(args, a)
			if p.take(",") != "" {
				continue
			}
			if p.take(")") == "" {
				return nil, fmt.Errorf("missing ) after the arguments of %s", fn)
			}
			break
		}
	}
	if len(args) < limits[0] || (limits[1] >= 0 && len(args) > limits[1]) {
		return nil, fmt.Errorf("%s takes %s", fn, argCount(limits))
	}
	return call{fn: fn, args: args}, nil
}

func argCount(l [2]int) string {
	switch {
	case l[1] < 0:
		return fmt.Sprintf("%d or more arguments", l[0])
	case l[0] == l[1]:
		return fmt.Sprintf("%d arguments", l[0])
	}
	return fmt.Sprintf("%d to %d arguments", l[0], l[1])
}
