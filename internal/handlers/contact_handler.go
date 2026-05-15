package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type ContactRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Message string `json:"message"`
}

type ContactResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (h *TravelHandler) ContactHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ContactResponse{Error: "method not allowed"})
		return
	}

	var req ContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ContactResponse{Error: "invalid request"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.Message = strings.TrimSpace(req.Message)

	if req.Name == "" || !strings.Contains(req.Email, "@") || req.Message == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(ContactResponse{Error: "all fields are required"})
		return
	}

	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ContactResponse{Error: "email service not configured"})
		return
	}

	toEmail := os.Getenv("CONTACT_TO_EMAIL")
	if toEmail == "" {
		toEmail = "bozgun76@gmail.com"
	}

	body := map[string]interface{}{
		"from":    "TravelMirror Contact <onboarding@resend.dev>",
		"to":      []string{toEmail},
		"subject": fmt.Sprintf("TravelMirror Contact: %s", req.Name),
		"html": fmt.Sprintf(`
			<p><strong>Name:</strong> %s</p>
			<p><strong>Email:</strong> %s</p>
			<p><strong>Message:</strong><br>%s</p>
		`, req.Name, req.Email, strings.ReplaceAll(req.Message, "\n", "<br>")),
	}

	payload, _ := json.Marshal(body)
	httpReq, _ := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(payload))
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil || resp.StatusCode >= 400 {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ContactResponse{Error: "failed to send email"})
		return
	}

	json.NewEncoder(w).Encode(ContactResponse{OK: true})
}
