package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

const mailjetAPI = "https://api.mailjet.com/v3.1/send"

type mjRecipient struct {
	Email string `json:"Email"`
	Name  string `json:"Name"`
}

type mjContent struct {
	From     mjRecipient   `json:"From"`
	To       []mjRecipient `json:"To"`
	Subject  string        `json:"Subject"`
	TextPart string        `json:"TextPart"`
	HTMLPart string        `json:"HTMLPart"`
}

type mjPayload struct {
	Messages []mjContent `json:"Messages"`
}

// sendNotification envoie l'email de notification à Pascal
func sendNotification(req ContactRequest) error {
	to := os.Getenv("NOTIFY_EMAIL")
	if to == "" {
		to = os.Getenv("SMTP_FROM")
	}

	categorieLabel := categorieToLabel(req.Categorie)

	subject := fmt.Sprintf("Nouveau contact : %s — %s", req.Prenom, categorieLabel)

	text := fmt.Sprintf(`Nouveau message reçu via le formulaire vivalink.top

Prénom      : %s
Email       : %s
Disponibilité : %s
Catégorie   : %s

— Journée type —
%s
`,
		req.Prenom,
		req.Email,
		valueOrDash(req.Disponibilite),
		categorieLabel,
		req.Journee,
	)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"/></head>
<body style="font-family:monospace;background:#0A0A0A;color:#F5F4F0;padding:40px;max-width:600px;margin:0 auto;">
  <div style="border-left:3px solid #FF4D00;padding-left:20px;margin-bottom:32px;">
    <p style="color:#FF4D00;font-size:12px;letter-spacing:2px;text-transform:uppercase;margin-bottom:8px;">Nouveau contact</p>
    <h1 style="font-size:28px;margin:0;">%s</h1>
  </div>
  <table style="width:100%%;border-collapse:collapse;">
    <tr><td style="padding:10px 0;color:#777;font-size:12px;letter-spacing:1px;text-transform:uppercase;width:140px;">Email</td><td style="padding:10px 0;"><a href="mailto:%s" style="color:#FF4D00;">%s</a></td></tr>
    <tr><td style="padding:10px 0;color:#777;font-size:12px;letter-spacing:1px;text-transform:uppercase;">Catégorie</td><td style="padding:10px 0;">%s</td></tr>
    <tr><td style="padding:10px 0;color:#777;font-size:12px;letter-spacing:1px;text-transform:uppercase;">Disponibilité</td><td style="padding:10px 0;">%s</td></tr>
  </table>
  <div style="margin-top:32px;border-top:1px solid rgba(255,255,255,0.07);padding-top:24px;">
    <p style="color:#777;font-size:12px;letter-spacing:1px;text-transform:uppercase;margin-bottom:12px;">Journée type</p>
    <p style="color:#F5F4F0;line-height:1.8;white-space:pre-wrap;">%s</p>
  </div>
</body>
</html>`,
		req.Prenom,
		req.Email, req.Email,
		categorieLabel,
		valueOrDash(req.Disponibilite),
		req.Journee,
	)

	return sendMail(to, "Pascal", subject, text, html)
}

// sendConfirmation envoie l'email de confirmation au visiteur
func sendConfirmation(req ContactRequest) error {
	subject := "On se parle bientôt — j'ai bien reçu votre message"

	text := fmt.Sprintf(`Bonjour %s,

J'ai bien reçu votre message et je le lis avec attention.

Je vous recontacte sous 24h pour qu'on cale un appel de 20 min — sans engagement, juste pour comprendre votre situation et voir si je peux vous aider.

En attendant : y a-t-il une contrainte importante que vous n'avez pas mentionnée ? (délai, budget, contexte particulier) Répondez simplement à cet email.

À très vite,
Pascal`,
		req.Prenom,
	)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"/></head>
<body style="font-family:'Helvetica Neue',sans-serif;background:#0A0A0A;color:#F5F4F0;padding:40px;max-width:600px;margin:0 auto;">
  <div style="margin-bottom:40px;">
    <span style="font-family:monospace;font-size:22px;letter-spacing:2px;font-weight:900;">PASCAL<span style="color:#FF4D00;">.</span></span>
  </div>
  <h1 style="font-size:32px;font-weight:900;line-height:1.1;margin-bottom:24px;">
    Bonjour %s,<br>
    <span style="color:#FF4D00;">c'est bien reçu.</span>
  </h1>
  <p style="color:#777;font-size:15px;line-height:1.8;margin-bottom:20px;">
    J'ai bien reçu votre message et je le lis avec attention.
  </p>
  <p style="color:#777;font-size:15px;line-height:1.8;margin-bottom:32px;">
    Je vous recontacte sous 24h pour qu'on cale un appel de 20 min — sans engagement, juste pour comprendre votre situation et voir si je peux vous aider.
  </p>
  <div style="border-left:2px solid #FF4D00;padding:16px 20px;margin-bottom:40px;">
    <p style="font-family:monospace;font-size:13px;color:#777;line-height:1.7;margin:0;">
      En attendant : y a-t-il une contrainte importante que vous n'avez pas mentionnée ?<br>
      (délai, budget, contexte particulier) Répondez simplement à cet email.
    </p>
  </div>
  <p style="color:#F5F4F0;font-size:15px;">À très vite,<br><strong>Pascal</strong></p>
  <div style="margin-top:48px;padding-top:24px;border-top:1px solid rgba(255,255,255,0.07);">
    <p style="font-family:monospace;font-size:11px;color:#444;letter-spacing:1px;">vivalink.top</p>
  </div>
</body>
</html>`,
		req.Prenom,
	)

	return sendMail(req.Email, req.Prenom, subject, text, html)
}

// sendMail appelle l'API Mailjet
func sendMail(toEmail, toName, subject, text, html string) error {
	apiKey := os.Getenv("SMTP_USER")
	apiSecret := os.Getenv("SMTP_PASS")
	fromEmail := os.Getenv("FROM_EMAIL")
	if fromEmail == "" {
		fromEmail = os.Getenv("SMTP_FROM")
	}
	fromName := os.Getenv("FROM_NAME")

	if fromEmail == "" {
		fromEmail = "pascal@vivalink.top"
	}
	if fromName == "" {
		fromName = "Pascal"
	}

	payload := mjPayload{
		Messages: []mjContent{
			{
				From: mjRecipient{Email: fromEmail, Name: fromName},
				To:   []mjRecipient{{Email: toEmail, Name: toName}},
				Subject:  subject,
				TextPart: text,
				HTMLPart: html,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, mailjetAPI, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("création requête: %w", err)
	}
	req.SetBasicAuth(apiKey, apiSecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("appel Mailjet: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Mailjet status %d", resp.StatusCode)
	}

	return nil
}

func categorieToLabel(cat string) string {
	labels := map[string]string{
		"taches-repetitives": "Tâches répétitives chronophages",
		"outil-manquant":     "Outil manquant introuvable",
		"process-brouillon":  "Process client brouillon",
		"intuition":          "Intuition — pas encore défini",
	}
	if l, ok := labels[cat]; ok {
		return l
	}
	return cat
}

func valueOrDash(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "—"
	}
	return s
}