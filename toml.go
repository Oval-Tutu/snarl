package snarl

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// The profile files use a small TOML subset: comments, bare keys, string,
// number and boolean values, [table] headers and [[array]] headers, with
// one level of nesting. Unknown keys are errors.

type tomlTable struct {
	values map[string]tomlValue
	tables map[string]*tomlTable
	arrays map[string][]*tomlTable
}

type tomlValue struct {
	line int
	str  string
	num  float64
	kind byte // 's' string, 'n' number, 'b' boolean.
}

func newTOMLTable() *tomlTable {
	return &tomlTable{
		values: map[string]tomlValue{},
		tables: map[string]*tomlTable{},
		arrays: map[string][]*tomlTable{},
	}
}

func isBareKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

// stripComment removes a # comment that is outside a string.
func stripComment(s string) string {
	in := false
	for i, r := range s {
		switch {
		case r == '"':
			in = !in
		case r == '#' && !in:
			return s[:i]
		}
	}
	return s
}

func parseTOMLValue(s string, line int) (tomlValue, error) {
	switch {
	case strings.HasPrefix(s, `"`):
		if len(s) < 2 || !strings.HasSuffix(s, `"`) || strings.ContainsAny(s[1:len(s)-1], "\"\\") {
			return tomlValue{}, fmt.Errorf("line %d: bad string %s", line, s)
		}
		return tomlValue{line: line, kind: 's', str: s[1 : len(s)-1]}, nil
	case s == "true" || s == "false":
		v := tomlValue{line: line, kind: 'b'}
		if s == "true" {
			v.num = 1
		}
		return v, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return tomlValue{}, fmt.Errorf("line %d: bad value %s", line, s)
	}
	return tomlValue{line: line, kind: 'n', num: f}, nil
}

func parseTOML(data string) (*tomlTable, error) {
	root := newTOMLTable()
	cur := root
	for i, raw := range strings.Split(data, "\n") {
		line := i + 1
		s := strings.TrimSpace(stripComment(raw))
		switch {
		case s == "":
			continue
		case strings.HasPrefix(s, "[["):
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "[["), "]]"))
			if !strings.HasSuffix(s, "]]") || !isBareKey(name) {
				return nil, fmt.Errorf("line %d: bad array header", line)
			}
			if _, ok := root.tables[name]; ok {
				return nil, fmt.Errorf("line %d: %s is a table", line, name)
			}
			cur = newTOMLTable()
			root.arrays[name] = append(root.arrays[name], cur)
		case strings.HasPrefix(s, "["):
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "["), "]"))
			if !strings.HasSuffix(s, "]") || !isBareKey(name) {
				return nil, fmt.Errorf("line %d: bad table header", line)
			}
			_, isTable := root.tables[name]
			_, isArray := root.arrays[name]
			if isTable || isArray {
				return nil, fmt.Errorf("line %d: %s is defined twice", line, name)
			}
			cur = newTOMLTable()
			root.tables[name] = cur
		default:
			k, v, ok := strings.Cut(s, "=")
			k = strings.TrimSpace(k)
			if !ok || !isBareKey(k) {
				return nil, fmt.Errorf("line %d: want key = value", line)
			}
			if _, dup := cur.values[k]; dup {
				return nil, fmt.Errorf("line %d: %s is defined twice", line, k)
			}
			val, err := parseTOMLValue(strings.TrimSpace(v), line)
			if err != nil {
				return nil, err
			}
			cur.values[k] = val
		}
	}
	return root, nil
}

// decodeTOML sets the fields of the struct that dst points to from the
// table. Fields keep their value when the table has no key for them.
func decodeTOML(t *tomlTable, dst any) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return errors.New("decode target is not a struct pointer")
	}
	return decodeStruct(t, v.Elem())
}

func decodeStruct(t *tomlTable, v reflect.Value) error {
	known := map[string]bool{}
	ty := v.Type()
	for i := range ty.NumField() {
		key := ty.Field(i).Tag.Get("toml")
		if key == "" {
			continue
		}
		known[key] = true
		if err := decodeField(t, key, v.Field(i)); err != nil {
			return err
		}
	}
	for k, val := range t.values {
		if !known[k] {
			return fmt.Errorf("line %d: unknown key %s", val.line, k)
		}
	}
	for k := range t.tables {
		if !known[k] {
			return fmt.Errorf("unknown table %s", k)
		}
	}
	for k := range t.arrays {
		if !known[k] {
			return fmt.Errorf("unknown array %s", k)
		}
	}
	return nil
}

func decodeField(t *tomlTable, key string, f reflect.Value) error {
	switch f.Kind() {
	case reflect.Struct:
		if sub, ok := t.tables[key]; ok {
			return decodeStruct(sub, f)
		}
		return nil
	case reflect.Slice:
		items := t.arrays[key]
		if len(items) == 0 {
			return nil
		}
		s := reflect.MakeSlice(f.Type(), len(items), len(items))
		for i, it := range items {
			if err := decodeStruct(it, s.Index(i)); err != nil {
				return err
			}
		}
		f.Set(s)
		return nil
	default:
	}
	val, ok := t.values[key]
	if !ok {
		return nil
	}
	switch f.Kind() {
	case reflect.String:
		if val.kind != 's' {
			return fmt.Errorf("line %d: %s must be a string", val.line, key)
		}
		f.SetString(val.str)
	case reflect.Float64:
		if val.kind != 'n' {
			return fmt.Errorf("line %d: %s must be a number", val.line, key)
		}
		f.SetFloat(val.num)
	case reflect.Int:
		if val.kind != 'n' || val.num != float64(int(val.num)) {
			return fmt.Errorf("line %d: %s must be an integer", val.line, key)
		}
		f.SetInt(int64(val.num))
	case reflect.Bool:
		if val.kind != 'b' {
			return fmt.Errorf("line %d: %s must be true or false", val.line, key)
		}
		f.SetBool(val.num != 0)
	default:
		return fmt.Errorf("field for %s has an unsupported type", key)
	}
	return nil
}
