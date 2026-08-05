package spec

// Rich options: an option can carry a label and a description without changing
// what it answers with.
//
//	options:
//	  - postgres                          # still the common case
//	  - value: cockroach
//	    label: CockroachDB
//	    description: distributed, wire-compatible with postgres
//
// The value is what the wizard answers with; the label is what the person
// reading the terminal sees; the description is the sentence that explains the
// choice. Splitting them matters because the two audiences are different: a
// caller wants `cockroach`, a human wants to know what it costs them.
//
// Options stay a []string of values, so every widget, validator and helper that
// already worked on them is unchanged and the answers cannot drift from what
// the spec declares. Labels and descriptions ride alongside, index-aligned,
// and are nil for a spec that never uses them — which is most specs, and the
// case that must not pay for this.

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// optionEntry is one entry of an `options:` list, in either form.
type optionEntry struct {
	Value       string `yaml:"value"`
	Label       string `yaml:"label"`
	Description string `yaml:"description"`
}

// UnmarshalYAML accepts a scalar (the value) or a mapping.
func (o *optionEntry) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		// Decoding into a string keeps numbers and booleans usable as options:
		// `options: [80, 443]` is a list of ports, not a type error.
		return node.Decode(&o.Value)
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("an option must be a value or a mapping with value/label/description")
	}
	type entry optionEntry // no methods, so no recursion
	var e entry
	if err := node.Decode(&e); err != nil {
		return err
	}
	*o = optionEntry(e)
	if o.Value == "" {
		// A mapping that only names a label still has an obvious value, and
		// rejecting it would only make the author type the same word twice.
		o.Value = o.Label
	}
	if o.Value == "" {
		return fmt.Errorf("an option mapping needs a value (or a label to use as one)")
	}
	return nil
}

// stepFields is Step without its methods, so decoding into it does not call
// Step.UnmarshalYAML again.
type stepFields Step

// UnmarshalYAML decodes a step, then decodes its options separately because
// they accept two shapes.
func (s *Step) UnmarshalYAML(node *yaml.Node) error {
	var fields stepFields
	if err := node.Decode(&fields); err != nil {
		return err
	}
	*s = Step(fields)

	var probe struct {
		Options []optionEntry `yaml:"options"`
	}
	if err := node.Decode(&probe); err != nil {
		return fmt.Errorf("step %q: %v", s.Key, err)
	}
	if len(probe.Options) == 0 {
		return nil
	}
	s.Options = make([]string, len(probe.Options))
	var labels, descriptions []string
	for i, e := range probe.Options {
		s.Options[i] = e.Value
		if e.Label != "" && e.Label != e.Value {
			if labels == nil {
				labels = make([]string, len(probe.Options))
			}
			labels[i] = e.Label
		}
		if e.Description != "" {
			if descriptions == nil {
				descriptions = make([]string, len(probe.Options))
			}
			descriptions[i] = e.Description
		}
	}
	s.Labels = labels
	s.Descriptions = descriptions
	return nil
}

// OptionLabel is what to draw for option i: its label when it has one, and
// otherwise the value itself.
func (s Step) OptionLabel(i int) string {
	if i >= 0 && i < len(s.Labels) && s.Labels[i] != "" {
		return s.Labels[i]
	}
	if i >= 0 && i < len(s.Options) {
		return s.Options[i]
	}
	return ""
}

// OptionDescription is the explanatory line for option i, or "".
func (s Step) OptionDescription(i int) string {
	if i >= 0 && i < len(s.Descriptions) {
		return s.Descriptions[i]
	}
	return ""
}

// OptionLabels returns the text to draw for every option, or nil when no option
// has a label — which lets a caller keep using s.Options directly and allocate
// nothing in the common case.
func (s Step) OptionLabels() []string {
	if len(s.Labels) == 0 {
		return nil
	}
	out := make([]string, len(s.Options))
	for i := range s.Options {
		out[i] = s.OptionLabel(i)
	}
	return out
}

// IndexOfOption finds an option by value or by label, ignoring case, so a
// person can type either the word they read on screen or the one the caller
// will receive. Returns -1 when neither matches.
func (s Step) IndexOfOption(text string) int {
	for i := range s.Options {
		if equalFold(s.Options[i], text) {
			return i
		}
	}
	for i := range s.Labels {
		if s.Labels[i] != "" && equalFold(s.Labels[i], text) {
			return i
		}
	}
	return -1
}

// clearOptionText drops labels and descriptions. A widget that rewrites a
// step's options — the file step builds a directory listing out of one — must
// call it, or the text would stay aligned to options that are gone.
func (s *Step) clearOptionText() {
	s.Labels = nil
	s.Descriptions = nil
}

// ReplaceOptions swaps a step's options and forgets any label or description
// that belonged to the old ones.
func (s *Step) ReplaceOptions(options []string) {
	s.Options = options
	s.clearOptionText()
}

// equalFold is strings.EqualFold under a local name, so option.go does not have
// to import strings for one call.
func equalFold(a, b string) bool { return strings.EqualFold(a, b) }
