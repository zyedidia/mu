package buffer

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Slicer interface {
	Slice(start, end int) []byte
}

// these functions adapt the 'expand' function from the regexp package to work
// on a generalized 'Slicer' interface so that it can be used with our fancy
// buffer data structure.
func expand(re *regexp.Regexp, dst []byte, template string, src Slicer, match []int) []byte {
	for len(template) > 0 {
		i := strings.Index(template, "$")
		if i < 0 {
			break
		}
		dst = append(dst, template[:i]...)
		template = template[i:]
		if len(template) > 1 && template[1] == '$' {
			// Treat $$ as $.
			dst = append(dst, '$')
			template = template[2:]
			continue
		}
		name, num, rest, ok := extract(template)
		if !ok {
			// Malformed; treat $ as raw text.
			dst = append(dst, '$')
			template = template[1:]
			continue
		}
		template = rest
		if num >= 0 {
			if 2*num+1 < len(match) && match[2*num] >= 0 {
				dst = append(dst, src.Slice(match[2*num], match[2*num+1])...)
			}
		} else {
			subexpnames := re.SubexpNames()
			for i, namei := range subexpnames {
				if name == namei && 2*i+1 < len(match) && match[2*i] >= 0 {
					dst = append(dst, src.Slice(match[2*i], match[2*i+1])...)
					break
				}
			}
		}
	}
	dst = append(dst, template...)
	return dst
}

// extract returns the name from a leading "$name" or "${name}" in str.
// If it is a number, extract returns num set to that number; otherwise num = -1.
func extract(str string) (name string, num int, rest string, ok bool) {
	if len(str) < 2 || str[0] != '$' {
		return
	}
	brace := false
	if str[1] == '{' {
		brace = true
		str = str[2:]
	} else {
		str = str[1:]
	}
	i := 0
	for i < len(str) {
		rune, size := utf8.DecodeRuneInString(str[i:])
		if !unicode.IsLetter(rune) && !unicode.IsDigit(rune) && rune != '_' {
			break
		}
		i += size
	}
	if i == 0 {
		// empty name is not okay
		return
	}
	name = str[:i]
	if brace {
		if i >= len(str) || str[i] != '}' {
			// missing closing brace
			return
		}
		i++
	}

	// Parse number.
	num = 0
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || '9' < name[i] || num >= 1e8 {
			num = -1
			break
		}
		num = num*10 + int(name[i]) - '0'
	}
	// Disallow leading zeros.
	if name[0] == '0' && len(name) > 1 {
		num = -1
	}

	rest = str[i:]
	ok = true
	return
}
