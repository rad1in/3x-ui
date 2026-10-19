package controller

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestShopTicketsFollowTheWalletProofAndKeepMarkupInert(t *testing.T) {
	h := newWalletHarness(t)
	open := `{"email":"alice","proof":"` + h.subID + `","subject":"<img src=x onerror=alert(1)>","body":"<script>alert(1)</script>"}`

	_, reply := h.call(t, http.MethodPost, "/shop/api/tickets", strings.Replace(open, h.subID, "wrong", 1))
	if reply.Success || reply.Msg != service.ErrProofInvalid {
		t.Fatalf("open with a wrong proof = %+v, want %q", reply, service.ErrProofInvalid)
	}
	_, reply = h.call(t, http.MethodPost, "/shop/api/tickets", open)
	var created service.TicketDetail
	if err := json.Unmarshal(reply.Obj, &created); err != nil || !reply.Success {
		t.Fatalf("open = %+v, %v", reply, err)
	}
	if created.Ticket.Subject != "<img src=x onerror=alert(1)>" || created.Messages[0].Body != "<script>alert(1)</script>" {
		t.Fatalf("ticket = %+v; the text must come back verbatim as data", created)
	}

	id := strconv.Itoa(created.Ticket.Id)
	auth := "?email=alice&proof=" + url.QueryEscape(h.subID)
	_, reply = h.call(t, http.MethodGet, "/shop/api/tickets"+auth, "")
	var list []model.Ticket
	if err := json.Unmarshal(reply.Obj, &list); err != nil || !reply.Success || len(list) != 1 {
		t.Fatalf("list = %+v, %v", reply, err)
	}
	_, reply = h.call(t, http.MethodPost, "/shop/api/tickets/"+id+"/reply",
		`{"email":"alice","proof":"`+h.subID+`","body":"still there"}`)
	var replied service.TicketDetail
	if err := json.Unmarshal(reply.Obj, &replied); err != nil || !reply.Success || len(replied.Messages) != 2 {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	_, reply = h.call(t, http.MethodGet, "/shop/api/tickets/"+id+"?email=alice&proof=wrong", "")
	if reply.Success || reply.Msg != service.ErrProofInvalid {
		t.Fatalf("read with a wrong proof = %+v, want %q", reply, service.ErrProofInvalid)
	}
}
