package signup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"sync"

	"github.com/example/fieldservice-email-cutover/internal/infrai"
)

const (
	AwaitingVerification = "awaiting_email_verification"
	ReadyForDispatch     = "ready_for_dispatch"
)

type Photo struct {
	URL     string `json:"url"`
	Caption string `json:"caption"`
}

type FollowUp struct {
	Required bool   `json:"required"`
	Note     string `json:"note"`
}

type WorkOrder struct {
	ID             string   `json:"id"`
	SiteAddress    string   `json:"site_address"`
	Photos         []Photo  `json:"photos"`
	DispatchStatus string   `json:"dispatch_status"`
	FollowUp       FollowUp `json:"technician_follow_up"`
}

type Registration struct {
	ID         string    `json:"id"`
	Email      string    `json:"email"`
	Technician string    `json:"technician"`
	Verified   bool      `json:"verified"`
	WorkOrder  WorkOrder `json:"work_order"`
	MessageID  string    `json:"message_id"`
	Token      string    `json:"-"`
}

type EmailSender interface {
	Send(context.Context, infrai.SendEmailRequest, string) (infrai.SendEmailResult, error)
}

type Flow struct {
	Sender    EmailSender
	PublicURL string
	mu        sync.RWMutex
	byToken   map[string]*Registration
}

func NewFlow(sender EmailSender, publicURL string) *Flow {
	return &Flow{Sender: sender, PublicURL: publicURL, byToken: make(map[string]*Registration)}
}

func (f *Flow) Register(ctx context.Context, registration Registration) (Registration, error) {
	if registration.ID == "" || registration.Email == "" || registration.WorkOrder.ID == "" {
		return Registration{}, errors.New("id, email, and work_order.id are required")
	}
	token, err := newToken()
	if err != nil {
		return Registration{}, err
	}
	registration.Token = token
	registration.Verified = false
	registration.WorkOrder.DispatchStatus = AwaitingVerification
	link := f.PublicURL + "/verify?token=" + token
	result, err := f.Sender.Send(ctx, infrai.SendEmailRequest{
		To:      registration.Email,
		Subject: "Verify your field-service account",
		HTML:    fmt.Sprintf("<p>Hello %s,</p><p><a href=\"%s\">Verify email</a> to release work order %s for dispatch.</p>", html.EscapeString(registration.Technician), html.EscapeString(link), html.EscapeString(registration.WorkOrder.ID)),
	}, "field-signup-"+registration.ID)
	if err != nil {
		return Registration{}, err
	}
	registration.MessageID = result.MessageID
	f.mu.Lock()
	f.byToken[token] = &registration
	f.mu.Unlock()
	return registration, nil
}

func (f *Flow) Verify(token string) (Registration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	registration, ok := f.byToken[token]
	if !ok {
		return Registration{}, errors.New("verification link is invalid")
	}
	registration.Verified = true
	registration.WorkOrder.DispatchStatus = ReadyForDispatch
	return *registration, nil
}

func newToken() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("create verification token: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
