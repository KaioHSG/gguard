package ggs

import (
	"fmt"
	"strings"
	"time"
)

func Parse(input string) (*Guard, error) {
	guard := &Guard{}
	lines := strings.Split(input, "\n")

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		verb, args := splitLine(line)
		switch verb {
		case "guard":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: guard requires a name", i+1)
			}
			guard.Name = args[0]

		case "version":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: version requires a value", i+1)
			}
			guard.Version = args[0]

		case "os":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: os requires a value", i+1)
			}
			guard.OS = args[0]

		case "watch":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: watch requires a path", i+1)
			}
			guard.Watch = args[0]

		case "debounce":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: debounce requires a duration", i+1)
			}
			d, err := time.ParseDuration(args[0])
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid debounce duration %q: %w", i+1, args[0], err)
			}
			guard.Debounce = d

		case "zip":
			guard.Commands = append(guard.Commands, Command{
				Verb: "zip",
				Args: args,
			})

		case "sync":
			guard.Commands = append(guard.Commands, Command{
				Verb: "sync",
				Args: args,
			})

		case "message":
			msg := strings.Join(args, " ")
			guard.Commands = append(guard.Commands, Command{
				Verb: "message",
				Args: []string{msg},
			})

		default:
			return nil, fmt.Errorf("line %d: unknown command %q", i+1, verb)
		}
	}

	return guard, nil
}

func splitLine(line string) (verb string, args []string) {
	tokens := tokenize(line)
	if len(tokens) == 0 {
		return "", nil
	}
	return tokens[0], tokens[1:]
}

func tokenize(input string) []string {
	var tokens []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for i := 0; i < len(input); i++ {
		ch := input[i]

		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if ch == '"' {
			inQuotes = !inQuotes
			continue
		}

		if !inQuotes && ch == ' ' {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}