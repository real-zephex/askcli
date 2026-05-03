package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	BASE_URL string = "https://api.agentmail.to/v0"
)

// types for threads
type ThreadsResponse struct {
	Count   int      `json:"count"`
	Threads []Thread `json:"threads"`
}

type Thread struct {
	OrganizationID    string         `json:"organization_id"`
	PodID             string         `json:"pod_id"`
	InboxID           string         `json:"inbox_id"`
	ThreadID          string         `json:"thread_id"`
	Labels            []string       `json:"labels"`
	Timestamp         string         `json:"timestamp"`
	ReceivedTimestamp string         `json:"received_timestamp"`
	SentTimestamp     string         `json:"sent_timestamp,omitempty"`
	Senders           []string       `json:"senders"`
	Recipients        []string       `json:"recipients"`
	Subject           string         `json:"subject"`
	Preview           string         `json:"preview"`
	LastMessageID     string         `json:"last_message_id"`
	MessageCount      int            `json:"message_count"`
	Size              int            `json:"size"`
	UpdatedAt         string         `json:"updated_at"`
	CreatedAt         string         `json:"created_at"`
	Messages          []EmailMessage `json:"messages"`
}

type EmailMessage struct {
	OrganizationID string            `json:"organization_id"`
	PodID          string            `json:"pod_id"`
	InboxID        string            `json:"inbox_id"`
	ThreadID       string            `json:"thread_id"`
	MessageID      string            `json:"message_id"`
	Labels         []string          `json:"labels"`
	Timestamp      time.Time         `json:"timestamp"`
	From           string            `json:"from"`
	To             []string          `json:"to"`
	Subject        string            `json:"subject"`
	Preview        string            `json:"preview"`
	Headers        map[string]string `json:"headers"`
	SMTPID         string            `json:"smtp_id"`
	Size           int               `json:"size"`
	UpdatedAt      time.Time         `json:"updated_at"`
	CreatedAt      time.Time         `json:"created_at"`
	Text           string            `json:"text"`
	HTML           string            `json:"html"`
	ExtractedText  string            `json:"extracted_text"`
	ExtractedHTML  string            `json:"extracted_html"`
}

// types for sending message
type SendMessageRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

type SendMessageResponse struct {
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id"`
}

type ReplyMessageRequest struct {
	To      string `json:"to"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
	ReplyTo string `json:"reply_to"`
}

// Forward Message
type ForwardMessageRequest struct {
	To string `json:"to"`
}

type AgentMailError struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

func getAgentMailAPIKey() (string, error) {
	key, exists := os.LookupEnv("AGENT_MAIL_API_KEY")
	if !exists {
		return "", fmt.Errorf("please set AGENT_MAIL_API_KEY environment variable")
	}
	return key, nil
}

func getInboxName() (string, error) {
	inbox, exists := os.LookupEnv("INBOX_NAME")
	if !exists {
		return "", fmt.Errorf("please set INBOX_NAME environment variable")
	}
	return inbox, nil
}

func urlJoiner(path string) string {
	joined, _ := url.JoinPath(BASE_URL, path)
	return joined
}

func requestCrafter(path string, method string, body io.Reader) (*http.Request, error) {
	apiKey, err := getAgentMailAPIKey()
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(method, urlJoiner(path), body)
	if err != nil {
		return nil, fmt.Errorf("error crafting request for AgentMail: %v", err)
	}
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	if method == http.MethodPost && body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	return request, nil
}

func requestMaker(request *http.Request) (*http.Response, error) {
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	return response, nil
}

func getThreads() (ThreadsResponse, error) {
	inbox, err := getInboxName()
	if err != nil {
		return ThreadsResponse{}, err
	}

	path := fmt.Sprintf("inboxes/%s/threads", url.QueryEscape(inbox))
	request, err := requestCrafter(path, "GET", nil)
	if err != nil {
		return ThreadsResponse{}, err
	}

	response, err := requestMaker(request)
	if err != nil {
		return ThreadsResponse{}, err
	}
	defer response.Body.Close()

	var threadsResponse ThreadsResponse
	if err := json.NewDecoder(response.Body).Decode(&threadsResponse); err != nil {
		return ThreadsResponse{}, err
	}

	return threadsResponse, nil
}

func getIndividualThread(threadID string) (Thread, error) {
	inbox, err := getInboxName()
	if err != nil {
		return Thread{}, err
	}

	path := fmt.Sprintf("inboxes/%s/threads/%s", url.QueryEscape(inbox), threadID)
	request, err := requestCrafter(path, "GET", nil)
	if err != nil {
		return Thread{}, err
	}

	response, err := requestMaker(request)
	if err != nil {
		return Thread{}, err
	}
	defer response.Body.Close()

	var thread Thread
	if err := json.NewDecoder(response.Body).Decode(&thread); err != nil {
		return Thread{}, err
	}

	return thread, nil
}

func sendEmail(to string, subject string, text string, html string) (SendMessageResponse, error) {
	inbox, err := getInboxName()
	if err != nil {
		return SendMessageResponse{}, err
	}

	payload := SendMessageRequest{
		To:      to,
		Subject: subject,
		Text:    text,
		HTML:    html,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("error marshaling send message request: %v", err)
	}

	path := fmt.Sprintf("inboxes/%s/messages/send", url.QueryEscape(inbox))
	request, err := requestCrafter(path, http.MethodPost, bytes.NewReader(data))
	if err != nil {
		return SendMessageResponse{}, err
	}

	response, err := requestMaker(request)
	if err != nil {
		return SendMessageResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return SendMessageResponse{}, fmt.Errorf("failed to send message, status: %d", response.StatusCode)
	}

	var sendResponse SendMessageResponse
	if err := json.NewDecoder(response.Body).Decode(&sendResponse); err != nil {
		return SendMessageResponse{}, err
	}

	return sendResponse, nil
}

func replyToMessage(messageID string, to string, text string, html string, replyTo string) (SendMessageResponse, error) {
	inbox, err := getInboxName()
	if err != nil {
		return SendMessageResponse{}, err
	}

	payload := ReplyMessageRequest{
		To:      to,
		Text:    text,
		HTML:    html,
		ReplyTo: replyTo,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("error marshaling reply request: %v", err)
	}

	path := fmt.Sprintf("inboxes/%s/messages/%s/reply", url.QueryEscape(inbox), url.QueryEscape(messageID))
	request, err := requestCrafter(path, http.MethodPost, bytes.NewReader(data))
	if err != nil {
		return SendMessageResponse{}, err
	}

	response, err := requestMaker(request)
	if err != nil {
		return SendMessageResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return SendMessageResponse{}, fmt.Errorf("failed to reply to message, status: %d", response.StatusCode)
	}

	var replyResponse SendMessageResponse
	if err := json.NewDecoder(response.Body).Decode(&replyResponse); err != nil {
		return SendMessageResponse{}, err
	}

	return replyResponse, nil
}

func forwardMessage(messageID string, to string) (SendMessageResponse, error) {
	inbox, err := getInboxName()
	if err != nil {
		return SendMessageResponse{}, err
	}

	payload := ForwardMessageRequest{To: to}

	data, err := json.Marshal(payload)
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("error marshaling forward request: %v", err)
	}

	path := fmt.Sprintf("inboxes/%s/messages/%s/forward", url.QueryEscape(inbox), url.QueryEscape(messageID))
	request, err := requestCrafter(path, http.MethodPost, bytes.NewReader(data))
	if err != nil {
		return SendMessageResponse{}, err
	}

	response, err := requestMaker(request)
	if err != nil {
		return SendMessageResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return SendMessageResponse{}, fmt.Errorf("failed to forward message, status: %d", response.StatusCode)
	}

	var forwardResponse SendMessageResponse
	if err := json.NewDecoder(response.Body).Decode(&forwardResponse); err != nil {
		return SendMessageResponse{}, err
	}

	return forwardResponse, nil
}

func deleteThread(threadID string) error {
	inbox, err := getInboxName()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("inboxes/%s/threads/%s", url.QueryEscape(inbox), threadID)
	request, err := requestCrafter(path, http.MethodDelete, nil)
	if err != nil {
		return err
	}

	response, err := requestMaker(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusAccepted {
		return nil
	}

	var agentMailError AgentMailError
	if err := json.NewDecoder(response.Body).Decode(&agentMailError); err != nil {
		return fmt.Errorf("request failed with status %d", response.StatusCode)
	}

	return fmt.Errorf("%s: %s", agentMailError.Name, agentMailError.Message)
}

const (
	maxMailTextLength = 4000
	maxMailHTMLLength = 4000
)

type mailRequest struct {
	Action    string
	ThreadID  string
	MessageID string
	To        string
	Subject   string
	Text      string
	HTML      string
	ReplyTo   string
}

type mailResult struct {
	Request       mailRequest
	Threads       []Thread
	Thread        *Thread
	MessageResult *SendMessageResponse
	ExecutionErr  string
	UserDenied    bool
}

func parseMailRequest(args map[string]any) (mailRequest, error) {
	if args == nil {
		return mailRequest{}, fmt.Errorf("function args missing")
	}

	action, err := requiredStringArg(args, "action")
	if err != nil {
		return mailRequest{}, err
	}

	req := mailRequest{
		Action: strings.ToLower(action),
	}

	switch req.Action {
	case "get_threads":
		return req, nil
	case "get_thread":
		threadID, err := requiredStringArg(args, "thread_id")
		if err != nil {
			return mailRequest{}, err
		}
		req.ThreadID = threadID
		return req, nil
	case "send_email":
		to, err := requiredStringArg(args, "to")
		if err != nil {
			return mailRequest{}, err
		}
		subject, err := requiredStringArg(args, "subject")
		if err != nil {
			return mailRequest{}, err
		}
		req.To = to
		req.Subject = subject
		req.Text = optionalStringArg(args, "text")
		req.HTML = optionalStringArg(args, "html")
		if req.Text == "" && req.HTML == "" {
			return mailRequest{}, fmt.Errorf("either text or html must be provided")
		}
		return req, nil
	case "reply_to_message":
		messageID, err := requiredStringArg(args, "message_id")
		if err != nil {
			return mailRequest{}, err
		}
		to, err := requiredStringArg(args, "to")
		if err != nil {
			return mailRequest{}, err
		}
		replyTo, err := requiredStringArg(args, "reply_to")
		if err != nil {
			return mailRequest{}, err
		}
		req.MessageID = messageID
		req.To = to
		req.ReplyTo = replyTo
		req.Text = optionalStringArg(args, "text")
		req.HTML = optionalStringArg(args, "html")
		if req.Text == "" && req.HTML == "" {
			return mailRequest{}, fmt.Errorf("either text or html must be provided")
		}
		return req, nil
	case "forward_message":
		messageID, err := requiredStringArg(args, "message_id")
		if err != nil {
			return mailRequest{}, err
		}
		to, err := requiredStringArg(args, "to")
		if err != nil {
			return mailRequest{}, err
		}
		req.MessageID = messageID
		req.To = to
		return req, nil
	case "delete_thread":
		threadID, err := requiredStringArg(args, "thread_id")
		if err != nil {
			return mailRequest{}, err
		}
		req.ThreadID = threadID
		return req, nil
	default:
		return mailRequest{}, fmt.Errorf("unknown action '%s': must be one of get_threads, get_thread, send_email, reply_to_message, forward_message, delete_thread", req.Action)
	}
}

func executeMail(req mailRequest) mailResult {
	res := mailResult{Request: req}

	switch req.Action {
	case "get_threads":
		threads, err := getThreads()
		if err != nil {
			res.ExecutionErr = err.Error()
			return res
		}
		res.Threads = threads.Threads
	case "get_thread":
		thread, err := getIndividualThread(req.ThreadID)
		if err != nil {
			res.ExecutionErr = err.Error()
			return res
		}
		res.Thread = &thread
	case "send_email":
		sendResult, err := sendEmail(req.To, req.Subject, req.Text, req.HTML)
		if err != nil {
			res.ExecutionErr = err.Error()
			return res
		}
		res.MessageResult = &sendResult
	case "reply_to_message":
		sendResult, err := replyToMessage(req.MessageID, req.To, req.Text, req.HTML, req.ReplyTo)
		if err != nil {
			res.ExecutionErr = err.Error()
			return res
		}
		res.MessageResult = &sendResult
	case "forward_message":
		sendResult, err := forwardMessage(req.MessageID, req.To)
		if err != nil {
			res.ExecutionErr = err.Error()
			return res
		}
		res.MessageResult = &sendResult
	case "delete_thread":
		if err := deleteThread(req.ThreadID); err != nil {
			res.ExecutionErr = err.Error()
			return res
		}
	}

	return res
}

func (res mailResult) toToolResponse() map[string]any {
	if res.ExecutionErr != "" {
		return map[string]any{
			"error": map[string]any{
				"message": res.ExecutionErr,
			},
		}
	}

	if res.UserDenied {
		return map[string]any{
			"error": map[string]any{
				"message": "operation denied by user",
			},
		}
	}

	response := map[string]any{
		"action": res.Request.Action,
	}

	switch res.Request.Action {
	case "get_threads":
		threads := make([]map[string]any, 0, len(res.Threads))
		for _, thread := range res.Threads {
			threads = append(threads, formatMailThreadSummary(thread))
		}
		response["threads"] = threads
		response["count"] = len(threads)
	case "get_thread":
		if res.Thread != nil {
			response["thread"] = formatMailThreadDetail(*res.Thread)
		}
	case "send_email", "reply_to_message", "forward_message":
		if res.MessageResult != nil {
			response["message"] = map[string]any{
				"message_id": res.MessageResult.MessageID,
				"thread_id":  res.MessageResult.ThreadID,
			}
		}
	case "delete_thread":
		response["thread_id"] = res.Request.ThreadID
		response["ok"] = true
	}

	return response
}

func mailActionNeedsApproval(action string) bool {
	switch action {
	case "send_email", "reply_to_message", "forward_message", "delete_thread":
		return true
	default:
		return false
	}
}

func askForMailApproval() bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Proceed with mail action? [y/N]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return false
		}
		v := strings.ToLower(strings.TrimSpace(line))
		switch v {
		case "y", "yes":
			return true
		case "", "n", "no":
			return false
		}
	}
}

func optionalStringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	value, ok := args[key]
	if !ok {
		return ""
	}
	strValue, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(strValue)
}

func truncateMailContent(value string, maxLen int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value, false
	}
	return string(runes[:maxLen]) + "...", true
}

func formatMailThreadSummary(thread Thread) map[string]any {
	return map[string]any{
		"thread_id":          thread.ThreadID,
		"subject":            thread.Subject,
		"preview":            thread.Preview,
		"message_count":      thread.MessageCount,
		"last_message_id":    thread.LastMessageID,
		"senders":            thread.Senders,
		"recipients":         thread.Recipients,
		"timestamp":          thread.Timestamp,
		"received_timestamp": thread.ReceivedTimestamp,
		"sent_timestamp":     thread.SentTimestamp,
		"updated_at":         thread.UpdatedAt,
		"created_at":         thread.CreatedAt,
	}
}

func formatMailThreadDetail(thread Thread) map[string]any {
	messages := make([]map[string]any, 0, len(thread.Messages))
	for _, message := range thread.Messages {
		messages = append(messages, formatMailMessage(message))
	}

	return map[string]any{
		"thread_id":          thread.ThreadID,
		"subject":            thread.Subject,
		"preview":            thread.Preview,
		"message_count":      thread.MessageCount,
		"last_message_id":    thread.LastMessageID,
		"senders":            thread.Senders,
		"recipients":         thread.Recipients,
		"timestamp":          thread.Timestamp,
		"received_timestamp": thread.ReceivedTimestamp,
		"sent_timestamp":     thread.SentTimestamp,
		"updated_at":         thread.UpdatedAt,
		"created_at":         thread.CreatedAt,
		"messages":           messages,
	}
}

func formatMailMessage(message EmailMessage) map[string]any {
	text, textTruncated := truncateMailContent(message.Text, maxMailTextLength)
	html, htmlTruncated := truncateMailContent(message.HTML, maxMailHTMLLength)
	extracted, extractedTruncated := truncateMailContent(message.ExtractedText, maxMailTextLength)

	payload := map[string]any{
		"message_id":     message.MessageID,
		"from":           message.From,
		"to":             message.To,
		"subject":        message.Subject,
		"preview":        message.Preview,
		"timestamp":      formatMailTime(message.Timestamp),
		"updated_at":     formatMailTime(message.UpdatedAt),
		"created_at":     formatMailTime(message.CreatedAt),
		"text":           text,
		"html":           html,
		"extracted_text": extracted,
	}

	if textTruncated {
		payload["text_truncated"] = true
	}
	if htmlTruncated {
		payload["html_truncated"] = true
	}
	if extractedTruncated {
		payload["extracted_text_truncated"] = true
	}

	return payload
}

func formatMailTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
