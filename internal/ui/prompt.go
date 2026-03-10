package ui

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Prompter struct {
	reader *bufio.Reader
	out    io.Writer
}

func NewPrompter(in io.Reader, out io.Writer) *Prompter {
	return &Prompter{
		reader: bufio.NewReader(in),
		out:    out,
	}
}

func (p *Prompter) AskChoice(label string, options []string, defaultValue string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("%s 没有可选项", label)
	}

	for {
		fmt.Fprintf(p.out, "%s\n", label)
		for i, option := range options {
			fmt.Fprintf(p.out, "  %d. %s\n", i+1, option)
		}
		if defaultValue != "" {
			fmt.Fprintf(p.out, "> [%s]: ", defaultValue)
		} else {
			fmt.Fprint(p.out, "> ")
		}

		text, err := p.readLine()
		if err != nil {
			return "", err
		}
		if text == "" && defaultValue != "" {
			return defaultValue, nil
		}
		if idx, err := strconv.Atoi(text); err == nil && idx >= 1 && idx <= len(options) {
			return options[idx-1], nil
		}
		for _, option := range options {
			if strings.EqualFold(text, option) {
				return option, nil
			}
		}
		fmt.Fprintf(p.out, "无效输入：%s\n\n", text)
	}
}

func (p *Prompter) AskString(label, defaultValue string, _ bool) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(p.out, "%s: ", label)
	}

	text, err := p.readLine()
	if err != nil {
		return "", err
	}
	if text == "" {
		return defaultValue, nil
	}
	return text, nil
}

func (p *Prompter) AskYesNo(label string, defaultValue bool) (bool, error) {
	defaultText := "默认否"
	if defaultValue {
		defaultText = "默认是"
	}

	for {
		fmt.Fprintf(p.out, "%s [%s]: ", label, defaultText)
		text, err := p.readLine()
		if err != nil {
			return false, err
		}
		if text == "" {
			return defaultValue, nil
		}

		switch strings.ToLower(text) {
		case "y", "yes", "shi", "是":
			return true, nil
		case "n", "no", "fou", "否":
			return false, nil
		default:
			fmt.Fprintf(p.out, "请输入 yes/no，也支持 y/n 或 是/否。\n")
		}
	}
}

func (p *Prompter) readLine() (string, error) {
	text, err := p.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(text), nil
}
