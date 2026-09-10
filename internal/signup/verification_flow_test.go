package signup

import (
	"context"
	"testing"

	"github.com/example/fieldservice-email-cutover/internal/infrai"
)

type recordingSender struct {
	request infrai.SendEmailRequest
	key     string
}

func (s *recordingSender) Send(_ context.Context, request infrai.SendEmailRequest, key string) (infrai.SendEmailResult, error) {
	s.request = request
	s.key = key
	return infrai.SendEmailResult{MessageID: "msg_42"}, nil
}

func TestVerificationControlsDispatch(t *testing.T) {
	tests := []struct {
		name         string
		verify       bool
		wantStatus   string
		wantVerified bool
	}{
		{name: "signup holds dispatch", wantStatus: AwaitingVerification, wantVerified: false},
		{name: "verified email releases dispatch", verify: true, wantStatus: ReadyForDispatch, wantVerified: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender := &recordingSender{}
			flow := NewFlow(sender, "http://localhost:8080")
			got, err := flow.Register(context.Background(), Registration{
				ID: "signup-42", Email: "tech@example.com", Technician: "Avery",
				WorkOrder: WorkOrder{ID: "WO-104", Photos: []Photo{{URL: "https://example.com/meter.jpg", Caption: "meter before repair"}}, FollowUp: FollowUp{Required: true, Note: "confirm pressure after 24 hours"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if test.verify {
				got, err = flow.Verify(got.Token)
				if err != nil {
					t.Fatal(err)
				}
			}
			if got.WorkOrder.DispatchStatus != test.wantStatus || got.Verified != test.wantVerified {
				t.Fatalf("got status=%q verified=%v", got.WorkOrder.DispatchStatus, got.Verified)
			}
			if sender.key != "field-signup-signup-42" || sender.request.To != "tech@example.com" {
				t.Fatalf("unexpected email boundary: key=%q to=%q", sender.key, sender.request.To)
			}
		})
	}
}
