package main

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"
)

const mailjetAPI = "https://api.mailjet.com/v3.1/send"

// headerSafe retire les retours à la ligne d'une valeur saisie par un visiteur : en SMTP,
// elle finit dans les en-têtes du message, où un \r\n permettrait d'en injecter d'autres.
var headerSafe = strings.NewReplacer("\r", " ", "\n", " ")

// transport décrit le mode d'envoi choisi par les variables d'environnement : SMTP dès
// que SMTP_HOST est renseigné, sinon l'API Mailjet (comportement historique).
func transport() string {
	if host := os.Getenv("SMTP_HOST"); host != "" {
		return "SMTP " + host + ":" + smtpPort()
	}
	return "API Mailjet"
}

func smtpPort() string {
	if p := os.Getenv("SMTP_PORT"); p != "" {
		return p
	}
	return "587"
}

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

// sendMail envoie via SMTP si SMTP_HOST est renseigné (ex. Brevo), sinon via l'API
// Mailjet. SMTP_USER/SMTP_PASS servent dans les deux cas : identifiants SMTP, ou clé et
// secret de l'API Mailjet.
func sendMail(toEmail, toName, subject, text, html string) error {
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

	if host := os.Getenv("SMTP_HOST"); host != "" {
		return sendSMTP(host, fromEmail, fromName, toEmail, toName, subject, text, html)
	}
	return sendMailjet(fromEmail, fromName, toEmail, toName, subject, text, html)
}

func sendSMTP(host, fromEmail, fromName, toEmail, toName, subject, text, html string) error {
	from, err := mail.ParseAddress(fromEmail)
	if err != nil {
		return fmt.Errorf("adresse d'expédition invalide: %w", err)
	}
	from.Name = headerSafe.Replace(fromName)
	to, err := mail.ParseAddress(strings.TrimSpace(toEmail))
	if err != nil {
		return fmt.Errorf("adresse destinataire invalide: %w", err)
	}
	to.Name = headerSafe.Replace(toName)

	msg, err := buildMessage(from, to, subject, text, html)
	if err != nil {
		return err
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, smtpPort()), 15*time.Second)
	if err != nil {
		return fmt.Errorf("connexion SMTP: %w", err)
	}
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("client SMTP: %w", err)
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}
	// PlainAuth refuse d'envoyer les identifiants sur une connexion non chiffrée.
	if user := os.Getenv("SMTP_USER"); user != "" {
		if err := c.Auth(smtp.PlainAuth("", user, os.Getenv("SMTP_PASS"), host)); err != nil {
			return fmt.Errorf("authentification SMTP: %w", err)
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("écriture du message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("fin du message: %w", err)
	}
	return c.Quit()
}

// buildMessage produit un email multipart/alternative (texte + HTML), en-têtes encodés
// en UTF-8 : le prénom et l'objet contiennent souvent des accents.
func buildMessage(from, to *mail.Address, subject, text, html string) ([]byte, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, part := range []struct{ ctype, content string }{
		{"text/plain; charset=utf-8", text},
		{"text/html; charset=utf-8", html},
	} {
		pw, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.ctype},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		qp := quotedprintable.NewWriter(pw)
		if _, err := qp.Write([]byte(part.content)); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	domain := from.Address[strings.LastIndex(from.Address, "@")+1:]

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", from.String())
	fmt.Fprintf(&msg, "To: %s\r\n", to.String())
	fmt.Fprintf(&msg, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", headerSafe.Replace(subject)))
	fmt.Fprintf(&msg, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&msg, "Message-ID: <%s@%s>\r\n", hex.EncodeToString(id), domain)
	msg.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", mw.Boundary())
	msg.Write(body.Bytes())
	return msg.Bytes(), nil
}

// sendMailjet appelle l'API Mailjet (SMTP_USER = clé API, SMTP_PASS = clé secrète).
func sendMailjet(fromEmail, fromName, toEmail, toName, subject, text, html string) error {
	apiKey := os.Getenv("SMTP_USER")
	apiSecret := os.Getenv("SMTP_PASS")

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