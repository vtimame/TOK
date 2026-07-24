package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func (c *CLI) runCompletion(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(opts.args) != 2 {
		return &UsageError{Message: "completion requires a shell: bash, zsh, or fish", Code: 2}
	}

	switch opts.args[1] {
	case "bash":
		fmt.Fprint(c.out, bashCompletionScript)
	case "zsh":
		fmt.Fprint(c.out, zshCompletionScript)
	case "fish":
		fmt.Fprint(c.out, fishCompletionScript)
	default:
		return &UsageError{Message: fmt.Sprintf("unknown completion shell %q", opts.args[1]), Code: 2}
	}
	return nil
}

func (c *CLI) runComplete(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	projectNames := c.completeProjectNames(ctx, opts)
	for _, candidate := range completionCandidates(opts.args[1:], projectNames) {
		fmt.Fprintln(c.out, candidate)
	}
	return nil
}

func (c *CLI) completeProjectNames(ctx context.Context, opts runtimeOptions) []string {
	if !completionNeedsProjects(opts.args[1:]) {
		return nil
	}

	_, _, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return nil
	}
	defer store.Close()

	projects, err := store.ListProjects(ctx)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(projects))
	for _, project := range projects {
		names = append(names, project.Name)
	}
	sort.Strings(names)
	return names
}

func completionNeedsProjects(args []string) bool {
	if len(args) == 0 {
		return false
	}
	previous := ""
	if len(args) >= 2 {
		previous = args[len(args)-2]
	}
	current := args[len(args)-1]
	return previous == "--project" || strings.HasPrefix(current, "--project=")
}

func completionCandidates(args []string, projectNames []string) []string {
	args = normalizeCompletionArgs(args)
	current := ""
	if len(args) > 0 {
		current = args[len(args)-1]
		args = args[:len(args)-1]
	}

	if valuePrefix, ok := completingFlagValue(args, current, "--project"); ok {
		return filterCandidates(projectNames, valuePrefix)
	}
	if values := flagValueChoices(args, current); len(values) > 0 {
		_, valuePrefix := splitFlagValue(current)
		return filterCandidates(values, valuePrefix)
	}

	spec := tokCommandSpec()
	for _, arg := range args {
		if isFlagToken(arg) {
			if flagTakesNextValue(spec, arg) {
				continue
			}
			continue
		}
		child := spec.child(arg)
		if child == nil {
			continue
		}
		spec = child
	}

	if current == "" && spec.Name == "completion" {
		return []string{"bash", "fish", "zsh"}
	}
	if strings.HasPrefix(current, "-") {
		return filterCandidates(flagCandidates(spec), current)
	}
	if children := childCandidates(spec); len(children) > 0 {
		return filterCandidates(children, current)
	}
	if spec.Name == "completion" {
		return filterCandidates([]string{"bash", "fish", "zsh"}, current)
	}
	return filterCandidates(flagCandidates(spec), current)
}

func normalizeCompletionArgs(args []string) []string {
	if len(args) == 0 {
		return []string{""}
	}
	normalized := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == commandName {
			continue
		}
		normalized = append(normalized, arg)
	}
	if len(normalized) == 0 {
		return []string{""}
	}
	return normalized
}

func completingFlagValue(args []string, current, flagName string) (string, bool) {
	if len(args) > 0 && args[len(args)-1] == flagName {
		return current, true
	}
	prefix := flagName + "="
	if strings.HasPrefix(current, prefix) {
		return strings.TrimPrefix(current, prefix), true
	}
	return "", false
}

func flagValueChoices(args []string, current string) []string {
	spec := commandSpecForCompletion(args)
	if len(args) > 0 {
		previous := args[len(args)-1]
		for _, flag := range spec.Flags {
			if flag.Name == previous && len(flag.ValueChoices) > 0 {
				return flag.ValueChoices
			}
		}
	}
	name, _ := splitFlagValue(current)
	for _, flag := range spec.Flags {
		if flag.Name == name && len(flag.ValueChoices) > 0 {
			return flag.ValueChoices
		}
	}
	if spec.ValuesByKey != nil {
		if values := spec.ValuesByKey["status"]; len(values) > 0 {
			return values
		}
	}
	return nil
}

func commandSpecForCompletion(args []string) *commandSpec {
	spec := tokCommandSpec()
	for _, arg := range args {
		if isFlagToken(arg) {
			continue
		}
		if child := spec.child(arg); child != nil {
			spec = child
		}
	}
	return spec
}

func splitFlagValue(value string) (string, string) {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return value, ""
	}
	return parts[0], parts[1]
}

func isFlagToken(value string) bool {
	return strings.HasPrefix(value, "-")
}

func flagTakesNextValue(spec *commandSpec, token string) bool {
	name, _ := splitFlagValue(token)
	for _, flag := range append(tokCommandSpec().Flags, spec.Flags...) {
		if flag.Name == name && flag.ValueName != "" && !strings.Contains(token, "=") {
			return true
		}
	}
	return false
}

func childCandidates(spec *commandSpec) []string {
	children := spec.visibleChildren()
	candidates := make([]string, 0, len(children))
	for _, child := range children {
		candidates = append(candidates, child.Name)
	}
	sort.Strings(candidates)
	return candidates
}

func flagCandidates(spec *commandSpec) []string {
	seen := map[string]bool{}
	flags := append([]flagSpec{}, tokCommandSpec().Flags...)
	flags = append(flags, spec.Flags...)
	candidates := make([]string, 0, len(flags))
	for _, flag := range flags {
		if seen[flag.Name] {
			continue
		}
		seen[flag.Name] = true
		candidates = append(candidates, flag.Name)
	}
	sort.Strings(candidates)
	return candidates
}

func filterCandidates(candidates []string, prefix string) []string {
	seen := map[string]bool{}
	filtered := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if seen[candidate] || !strings.HasPrefix(candidate, prefix) {
			continue
		}
		seen[candidate] = true
		filtered = append(filtered, candidate)
	}
	sort.Strings(filtered)
	return filtered
}

const bashCompletionScript = `_tok_completion() {
  local IFS=$'\n'
  local -a tok_words=("${COMP_WORDS[@]:1:COMP_CWORD}")
  if [[ "${COMP_LINE: -1}" == " " ]]; then
    tok_words+=("")
  fi
  COMPREPLY=($(tok __complete "${tok_words[@]}"))
}
complete -F _tok_completion tok
`

const zshCompletionScript = `#compdef tok
_tok() {
  local -a completions
  local -a tok_words
  tok_words=("${words[@]:1}")
  if [[ "${BUFFER[-1]}" == " " ]]; then
    tok_words+=("")
  fi
  completions=("${(@f)$(tok __complete "${tok_words[@]}")}")
  compadd -- $completions
}
compdef _tok tok
`

const fishCompletionScript = `function __tok_complete
  set -l tokens (commandline -opc)
  if test (count $tokens) -le 1
    tok __complete ""
  else
    tok __complete $tokens[2..-1]
  end
end
complete -c tok -f -a "(__tok_complete)"
`
