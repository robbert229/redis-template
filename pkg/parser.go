package pkg

import (
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/garyburd/redigo/redis"
)

// TemplateFlag denotes a templating action to perform. It has a source, the template to process. A target, the file to
// write to. And an action, a command to execute once the template has been updated.
type TemplateFlag struct {
	Source string
	Target string
	Action string
}

func (t TemplateFlag) ToTemplate(p redis.Pool) (Template, error) {
	sourceContents, err := ioutil.ReadFile(t.Source)
	if err != nil {
		return Template{}, err
	}

	temp, err := template.New(t.Source).Funcs(template.FuncMap{
		"key": func(argument interface{}) (interface{}, error) {
			key, ok := argument.(string)
			if !ok {
				return nil, errors.New("invalid argument given to key")
			}

			c, err := p.Dial()
			if err != nil {
				return nil, err
			}

			reply, err := redis.String(c.Do("GET", key))
			if err != nil {
				return nil, err
			}

			if err := c.Close(); err != nil {
				return nil, err
			}

			return reply, nil
		},
	}).Parse(string(sourceContents))
	if err != nil {
		return Template{}, err
	}

	return Template{
		SourceTemplate: temp,
		Target:         t.Target,
		Action:         t.Action,
	}, nil
}

// ParseTemplateFlag parses templates from strings.
func ParseTemplateFlag(input string) (TemplateFlag, error) {
	firstColon := strings.IndexRune(input, ':')
	if firstColon == -1 {
		return TemplateFlag{}, errors.New("invalid template given")
	}

	secondColon := strings.IndexRune(input[firstColon+1:], ':')
	if secondColon == -1 {
		return TemplateFlag{
			Source: input[0:firstColon],
			Target: input[firstColon+1:],
		}, nil
	} else {
		secondColon += firstColon + 1
	}

	return TemplateFlag{
		Source: input[0:firstColon],
		Target: input[firstColon+1 : secondColon],
		Action: input[secondColon+1:],
	}, nil
}

// Template is a processed version of a TemplateFlag. Instead of having a path to a
// source, it has the contents of the source. Otherwise it is the same as a TemplateFlag.
type Template struct {
	SourceTemplate *template.Template
	Target         string
	Action         string
}

// Execute executes the command
func (t Template) Execute() error {
	cmd := exec.Command("sh", "-c", t.Action)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return err
}
