package pkg

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/garyburd/redigo/redis"
)

// RedisTemplateChannel is the default channel in which redis-template will notify/listen that changes have been made
// upon. Redis doesn't have a watch mechanism we decided to not use polling to implement the redis template, and instead
// rely upon redis's pub sub feature.
const RedisTemplateChannel = "redis-template-channel"

// Config is the configuration that redis-template uses to perform its templating operations.
type Config struct {
	Logger    *log.Logger
	Pool      *redis.Pool
	Templates []Template
	Splay     time.Duration
	Channel   string
}

// TemplateFlags is a
type TemplateFlags []TemplateFlag

// Set implements the flag.Value interface's Set function. It takes a string from the cli arg and has it parsed.
func (t *TemplateFlags) Set(value string) error {
	newTemplate, err := ParseTemplateFlag(value)
	if err != nil {
		return errors.WithStack(err)
	}

	*t = append(*t, newTemplate)
	return nil
}

// String implements flag.Value interface's String function. It is a pretty print form of the flags
func (t *TemplateFlags) String() string {
	buffer := bytes.NewBuffer(nil)
	for _, tmpl := range *t {
		buffer.WriteString(tmpl.String())
	}

	return buffer.String()
}

// TemplateFlag denotes a templating action to perform. It has a source, the template to process. A target, the file to
// write to. And an action, a command to execute once the template has been updated.
type TemplateFlag struct {
	Source string
	Target string
	Action string
}

// String prints the original source of the template flag.
func (t TemplateFlag) String() string {
	if t.Action == "" {
		return fmt.Sprintf("%s:%s", t.Source, t.Target)
	}

	return fmt.Sprintf("%s:%s:%s", t.Source, t.Target, t.Action)
}

func (t TemplateFlag) ToTemplate(p *redis.Pool) (Template, error) {
	sourceContents, err := ioutil.ReadFile(t.Source)
	if err != nil {
		return Template{}, err
	}

	temp, err := template.New(t.Source).Funcs(template.FuncMap{
		"keyOrDefault": func(keyInterface interface{}, defaultValue interface{}) (interface{}, error) {
			key, ok := keyInterface.(string)
			if !ok {
				return nil, errors.New("invalid argument given to key")
			}

			c, err := p.Dial()
			if err != nil {
				return nil, err
			}

			reply, err := redis.String(c.Do("GET", key))
			if err != nil {
				if err == redis.ErrNil {
					return defaultValue, errors.WithStack(c.Close())
				}

				return nil, err
			}

			if err := c.Close(); err != nil {
				return nil, err
			}

			return reply, nil
		},
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
		Action: func() error {
			cmd := exec.Command("sh", "-c", t.Action)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err := cmd.Run()
			return errors.WithStack(err)
		},
	}, nil
}

// ParseTemplateFlag parses Templates from strings.
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
	Action         func() error
}

// Execute executes the command
func (t Template) Execute() error {
	return errors.WithStack(t.Action())
}
