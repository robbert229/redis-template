package pkg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var parserTestCases = []struct {
	Name     string
	Input    string
	Template TemplateFlag
	Fail     bool
}{
	{
		Name:  "no colon",
		Input: "foobar",
		Fail:  true,
	},
	{
		Name:  "basic source, and target",
		Input: "foo:bar",
		Template: TemplateFlag{
			Source: "foo",
			Target: "bar",
		},
		Fail: false,
	},
	{
		Name:  "basic source, target, and action",
		Input: "foo:bar:baz",
		Template: TemplateFlag{
			Source: "foo",
			Target: "bar",
			Action: "baz",
		},
		Fail: false,
	},
	{
		Name:  "basic source with empty target",
		Input: "foo:",
		Fail:  false,
		Template: TemplateFlag{
			Source: "foo",
			Target: "",
		},
	},
	{
		Name:     "empty source, empty target",
		Input:    ":",
		Template: TemplateFlag{},
	},
	{
		Name:     "empty source, empty target, empty action",
		Input:    "::",
		Template: TemplateFlag{},
	},
	{
		Name:  "realistic source, and target",
		Input: "/app/propman.json.tpl:/app/propman.json",
		Template: TemplateFlag{
			Source: "/app/propman.json.tpl",
			Target: "/app/propman.json",
		},
	},
	{
		Name:  "realistic source, target, and action",
		Input: "/app/propman.json.tpl:/app/propman.json:/app/update.sh",
		Template: TemplateFlag{
			Source: "/app/propman.json.tpl",
			Target: "/app/propman.json",
			Action: "/app/update.sh",
		},
	},
}

func TestParser(t *testing.T) {
	for _, tc := range parserTestCases {
		t.Run(tc.Name, func(t *testing.T) {
			template, err := ParseTemplateFlag(tc.Input)
			if !tc.Fail {
				if err != nil {
					t.Fatal(err)
				}

				if template != tc.Template {
					t.Fatalf("output didn't match expected. expected: %+v, actual: %+v", tc.Template, template)
				}
			} else {
				if err == nil {
					t.Fatal("Parse didn't return any errors")
				}
			}
		})
	}
}

func TestTemplateFlags_Set(t *testing.T) {
	flags := TemplateFlags{}
	if err := flags.Set("foo:bar:baz"); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, flags, TemplateFlags([]TemplateFlag{{
		Source: "foo",
		Target: "bar",
		Action: "baz",
	}}))

	if err := flags.Set("foo2:bar2:baz2"); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, flags, TemplateFlags([]TemplateFlag{{
		Source: "foo",
		Target: "bar",
		Action: "baz",
	}, {
		Source: "foo2",
		Target: "bar2",
		Action: "baz2",
	}}))

	if err := flags.Set("foo3:bar3"); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, flags, TemplateFlags([]TemplateFlag{{
		Source: "foo",
		Target: "bar",
		Action: "baz",
	}, {
		Source: "foo2",
		Target: "bar2",
		Action: "baz2",
	}, {
		Source: "foo3",
		Target: "bar3",
	}}))

	assert.NotNil(t, flags.Set("foo"))
}

func TestTemplateFlags_String(t *testing.T) {
	flags := TemplateFlags([]TemplateFlag{{
		Source: "foo",
		Target: "bar",
		Action: "baz",
	}, {
		Source: "foo2",
		Target: "bar2",
	}})

	assert.NotEmpty(t, flags.String())
}
