package pkg

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"sync"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// subscribe listens to redis waiting for messages to be published. Any errors encountered are sent to the errorsOut
// channel. When a message is returned from redis it is set to the messagesOut channel.
func subscribe(cfg Config, messagesOut chan redis.Message, errorsOut chan error) {
	// the pubsub subscriber needs to be in its own connection. Redis prevents connections subscribed to a channel to
	// from doing anything besides the channel operations.
	c, err := cfg.Pool.Dial()
	if err != nil {
		errorsOut <- err
		return
	}

	psc := &redis.PubSubConn{Conn: c}
	if err := psc.Subscribe(RedisTemplateChannel); err != nil {
		errorsOut <- err
		return
	}

	cfg.Logger.WithField("channel", RedisTemplateChannel).Info("subscribed to redis channel")

	for {
		reply := psc.Receive()
		cfg.Logger.WithField("reply", reply).Info("message received from redis")

		switch v := reply.(type) {
		case redis.Message:
			messagesOut <- v
		case error:
			errorsOut <- v
			return
		}
	}
}

// update renders the templates, and waits.
func update(cfg Config, previousTemplateExecutions map[string]string, mut sync.Locker) error {
	cfg.Logger.Debug("reloading Templates")

	// wait for a random time from 0 seconds up to the duration specified by splay.
	splayMs := int64(cfg.Splay / time.Millisecond)
	randomWaitMS := int64(rand.Float64() * float64(splayMs))
	randomWait := time.Duration(randomWaitMS) * time.Millisecond
	cfg.Logger.Debug("splay sleeping for: ", randomWait)
	time.Sleep(randomWait)

	// iterate over all of the templates and execute them. If any of them have changed, write the new templated
	// file to disk and perform the action (if it exists).
	for _, template := range cfg.Templates {
		cfg.Logger.Debug("executing template: ", template.SourceTemplate)
		if err := executeTemplate(template, cfg.Logger, previousTemplateExecutions, mut); err != nil {
			cfg.Logger.WithError(err).Error("failed to execute the template")
			return errors.WithStack(err)
		}
	}

	return nil
}

// Listen listens to the redis pubsub channel and when it detects any changes it will rerun all of its templates. If
// the results of the templates have changed then the new templated results is written to disk and the templates action
// is performed. If the template target is nil then the results are not persisted to disk.
func Listen(cfg Config) error {
	// previousTemplateExecutions is a map containing the results of previous template executions. It is used to detect
	// if a template has changed, in which case the template is written to disk and the action is performed.
	previousTemplateExecutions := map[string]string{}

	mut := &sync.Mutex{}

	// perform the initial execution; building all of the templates, writing all to disk, and executing all possible
	// actions.
	for _, template := range cfg.Templates {
		if err := executeTemplate(template, cfg.Logger, previousTemplateExecutions, mut); err != nil {
			return errors.WithStack(err)
		}
	}

	messageChan := make(chan redis.Message)
	errorChan := make(chan error)

	go subscribe(cfg, messageChan, errorChan)

	for {
		select {
		case <-messageChan:
			if err := update(cfg, previousTemplateExecutions, mut); err != nil {
				cfg.Logger.WithError(err).Error("fatal error occurred updated templates")
				return errors.WithStack(err)
			}
		case err := <-errorChan:
			cfg.Logger.WithError(err).Error("fatal error encountered in subscription")
			return errors.WithStack(err)
		}
	}
}

// executeTemplate executes the specified template, writes its output to the specified file, and then executes the
// action. All these actions are blocking. The given mutex synchronizes access to the previousTemplateExecutions map.
func executeTemplate(template Template, logger *log.Logger, previousTemplateExecutions map[string]string, mut sync.Locker) error {
	key := template.SourceTemplate.Name()

	logger.WithField("template", key).Info("executing template")

	buffer := bytes.NewBuffer(nil)
	if err := template.SourceTemplate.Execute(buffer, nil); err != nil {
		return err
	}

	mut.Lock()
	previousValue := previousTemplateExecutions[key]
	mut.Unlock()

	if previousValue != buffer.String() {
		// if there is a template target.
		if template.Target != nil {
			if err := ioutil.WriteFile(*template.Target, buffer.Bytes(), 0666); err != nil {
				return errors.WithStack(err)
			}
		}

		err := template.Execute()
		if err != nil {
			return errors.WithStack(err)
		}

		mut.Lock()
		previousTemplateExecutions[key] = buffer.String()
		mut.Unlock()
	}

	return nil
}
