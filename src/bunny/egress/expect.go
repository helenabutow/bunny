package egress

import (
	"bufio"
	"bunny/config"
	"fmt"
	"net"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

type ExpectStep interface {
	do(readWriter *bufio.ReadWriter, span *trace.Span) (bool, error)
}

type SendStep struct {
	text      string
	delimiter byte
}

type ReceiveStep struct {
	regex     *regexp.Regexp
	delimiter byte
}

func newExpectStep(expectStepConfig *config.ExpectConfig) ExpectStep {
	if expectStepConfig.Send != nil && expectStepConfig.Receive != nil {
		logger.Error("both send and receive set in single step of tcp socket action")
		return nil
	}
	if expectStepConfig.Send != nil {
		if len(expectStepConfig.Send.Delimiter) != 1 {
			logger.Error("expect step delimiters must be a length one string")
			return nil
		}
		text := expectStepConfig.Send.Text
		byteSlice := []byte(expectStepConfig.Send.Delimiter)
		step := SendStep{
			text:      text,
			delimiter: byteSlice[0],
		}
		return step
	} else {
		if len(expectStepConfig.Receive.Delimiter) != 1 {
			logger.Error("expect step delimiters must be a length one string")
			return nil
		}
		regexString := expectStepConfig.Receive.RegEx
		regex, err := regexp.Compile(regexString)
		if err != nil {
			logger.Error("error in regex for tcp socket action",
				"expectStep.Receive.RegEx", expectStepConfig.Receive.RegEx)
			return nil
		}
		byteSlice := []byte(expectStepConfig.Receive.Delimiter)
		var receiveStep = ReceiveStep{
			regex:     regex,
			delimiter: byteSlice[0],
		}
		return &receiveStep
	}
}

func expect(tcpConnection *net.Conn, steps []ExpectStep, span *trace.Span) bool {
	reader := bufio.NewReader(*tcpConnection)
	writer := bufio.NewWriter(*tcpConnection)
	readWriter := bufio.NewReadWriter(reader, writer)
	for _, step := range steps {
		successful, err := step.do(readWriter, span)
		if err != nil || !successful {
			return false
		}
	}
	return true
}

func (step SendStep) do(readWriter *bufio.ReadWriter, span *trace.Span) (bool, error) {
	logger.Debug("send step begins", "step.text", step.text)
	(*span).AddEvent(step.text)
	writer := readWriter.Writer
	fullText := step.text + string(step.delimiter)
	for i := 0; i < len(fullText); {
		remainingText := fullText[i:]
		bytesWritten, err := fmt.Fprint(writer, remainingText)
		if bytesWritten == 0 {
			logger.Debug("send step fails - no bytes written")
			return false, err
		}
		i += bytesWritten
	}
	err := writer.Flush()
	if err != nil {
		logger.Debug("send step fails - error during flush", "err", err)
		return false, err
	}
	logger.Debug("send step succeeds")
	return true, nil
}

func (step ReceiveStep) do(readWriter *bufio.ReadWriter, span *trace.Span) (bool, error) {
	logger.Debug("receive step begins", "step.regex.String()", step.regex.String())
	(*span).AddEvent(step.regex.String())
	wordsSent, err := readWriter.Reader.ReadString(step.delimiter)
	if err != nil {
		logger.Debug("receive step fails", "err", err)
		return false, err
	}
	trimmedWords := strings.TrimSuffix(wordsSent, string(step.delimiter))
	result := step.regex.MatchString(trimmedWords)
	logger.Debug("receive step result", "result", result)
	return result, nil
}
