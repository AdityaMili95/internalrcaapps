package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

const (
	DevelopmentEnv = "development"
)

var (
	colors = []string{"#1abc9c", "#2ecc71", "#3498db", "#f1c40f", "#e67e22", "#e74c3c", "#9b59b6", "#ecf0f1", "#34495e"}
)

type SlackModule struct {
	Webhook     string
	Color       string
	Client      *http.Client
	Environment string
}

func NewSlackModule(webHook, envi string) *SlackModule {
	slackM := &SlackModule{
		Webhook:     webHook,
		Environment: envi,
		Client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}

	slackM.RandomColor()
	return slackM
}

func (s *SlackModule) RandomColor() {
	rand.Seed(time.Now().Unix())
	color := colors[rand.Intn(len(colors))]
	s.Color = color
}

type SlackMsgStructure struct {
	Blocks []BlockStructure `json:"blocks"`
}

type BlockStructure struct {
	Type      string     `json:"type,omitempty"`
	Text      *BlockText `json:"text,omitempty"`
	Accessory *BlockAcc  `json:"accessory,omitempty"`
}

type BlockText struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

type BlockAcc struct {
	Type  string       `json:"type,omitempty"`
	Text  BlockAccText `json:"text,omitempty"`
	Value string       `json:"value,omitempty"`
}

type BlockAccText struct {
	Type  string `json:"type,omitempty"`
	Text  string `json:"text,omitempty"`
	Emoji bool   `json:"emoji,omitempty"`
}

func (s *SlackModule) PublishSlack(slackMsg SlackMsgStructure) error {
	if s.Environment == DevelopmentEnv {
		return nil
	}

	b, _ := json.Marshal(slackMsg)

	resp, err := s.Client.Post(s.Webhook, "application/json", bytes.NewBuffer(b))
	if err != nil {
		fmt.Println(err)
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		fmt.Printf("%+v", resp)
		return errors.New(fmt.Sprintf("Invalid code got: %d", resp.StatusCode))
	}
	return nil
}
