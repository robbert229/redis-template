package pkg

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Listen listens to the redis pubsub channel and when it detects any changes it will rerun all of its templates. If
// the results of the templates have changed then the new templated results is written to disk and the templates action
// is performed.
func Listen(cfg Config) error {
	// previousTemplateExecutions is a map containing the results of previous template executions. It is used to detect
	// if a template has changed, in which case the template is written to disk and the action is performed.
	previousTemplateExecutions := map[string]string{}

	// the pubsub subscriber needs to be in its own connection. Redis prevents connections subscribed to a channel to
	// from doing anything besides the channel operations.
	c, err := cfg.Pool.Dial()
	if err != nil {
		return errors.WithStack(err)
	}

	psc := &redis.PubSubConn{Conn: c}
	if err := psc.Subscribe(RedisTemplateChannel); err != nil {
		return errors.WithStack(err)
	}

	// parse all of the templates and anchor the redis pool into scope.
	templates := make([]Template, len(cfg.TemplateFlags))
	for i, templateFlag := range cfg.TemplateFlags {
		tmpl, err := templateFlag.ToTemplate(cfg.Pool)
		if err != nil {
			cfg.Logger.WithError(err).Errorf("failed build template")
			return errors.WithStack(err)
		}

		templates[i] = tmpl
	}

	// perform the initial execution; building all of the templates, writing all to disk, and executing all possible
	// actions.
	for _, template := range templates {
		if err := executeTemplate(template, cfg.Logger, previousTemplateExecutions); err != nil {
			return errors.WithStack(err)
		}
	}

	cfg.Logger.Info("subscribed to: ", RedisTemplateChannel)

	for {
		reply := psc.Receive()
		cfg.Logger.WithField("reply", reply).Info("message received from redis")

		switch v := reply.(type) {
		case redis.Message:
			cfg.Logger.Debug("reloading Templates")

			// wait for a random time from 0 seconds up to the duration specified by splay.
			splayMs := int64(cfg.Splay / time.Millisecond)
			randomWaitMS := int64(rand.Float64() * float64(splayMs))
			randomWait := time.Duration(randomWaitMS) * time.Millisecond
			cfg.Logger.Debug("splay sleeping for: ", randomWait)
			time.Sleep(randomWait)

			// iterate over all of the templates and execute them. If any of them have changed, write the new templated
			// file to disk and perform the action (if it exists).
			for _, template := range templates {
				cfg.Logger.Debug("executing template: ", template.SourceTemplate)
				if err := executeTemplate(template, cfg.Logger, previousTemplateExecutions); err != nil {
					cfg.Logger.WithError(err).Error("failed to execute the template")
					continue
				}
			}
		case error:
			cfg.Logger.WithError(v).Error("failed to receive message from redis")
		}
	}
}

// executeTemplate executes the specified template, writes its output to the specified file, and then executes the
// action. All these actions are blocking.
func executeTemplate(template Template, logger *log.Logger, previousTemplateExecutions map[string]string) error {
	key := template.SourceTemplate.Name()

	logger.WithField("template", key).Info("executing template")

	buffer := bytes.NewBuffer(nil)
	if err := template.SourceTemplate.Execute(buffer, nil); err != nil {
		return err
	}

	defer func() {
		previousTemplateExecutions[key] = buffer.String()
	}()

	if previousTemplateExecutions[key] != buffer.String() {
		if err := ioutil.WriteFile(template.Target, buffer.Bytes(), 0666); err != nil {
			return err
		}

		err := template.Execute()
		return errors.WithStack(err)
	}

	return nil
}
