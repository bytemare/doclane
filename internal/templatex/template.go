package templatex

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var placeholderRe = regexp.MustCompile(`\{\{\s*shared\.([A-Za-z0-9_]+)\s*\}\}`)

// Result contains the rendered template output and the variables that were used.
type Result struct {
	Rendered []byte
	UsedVars []string
}

// Render performs strict placeholder substitution for `{{ shared.<name> }}` tokens.
func Render(input []byte, vars map[string]string, strict bool) (Result, error) {
	if !utf8.Valid(input) {
		return Result{}, errors.New("template input is not valid UTF-8")
	}
	text := string(input)
	used := make(map[string]struct{})
	var missing []string

	out := placeholderRe.ReplaceAllStringFunc(text, func(match string) string {
		m := placeholderRe.FindStringSubmatch(match)
		if len(m) != 2 {
			return match
		}
		key := m[1]
		val, ok := vars[key]
		if !ok {
			missing = append(missing, key)
			return match
		}
		used[key] = struct{}{}
		return val
	})

	if len(missing) > 0 && strict {
		sort.Strings(missing)
		return Result{}, fmt.Errorf("missing template vars: %s", strings.Join(uniqueStrings(missing), ", "))
	}

	if strict {
		var unused []string
		for k := range vars {
			if _, ok := used[k]; !ok {
				unused = append(unused, k)
			}
		}
		if len(unused) > 0 {
			sort.Strings(unused)
			return Result{}, fmt.Errorf("unused template vars: %s", strings.Join(unused, ", "))
		}
	}

	usedList := make([]string, 0, len(used))
	for k := range used {
		usedList = append(usedList, k)
	}
	sort.Strings(usedList)

	return Result{
		Rendered: []byte(out),
		UsedVars: usedList,
	}, nil
}

func uniqueStrings(in []string) []string {
	set := make(map[string]struct{}, len(in))
	for _, s := range in {
		set[s] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
