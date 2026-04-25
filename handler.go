package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

type ContactRequest struct {
	Journee       string `json:"journee"`
	Categorie     string `json:"categorie"`
	Prenom        string `json:"prenom"`
	Email         string `json:"email"`
	Disponibilite string `json:"disponibilite"`
}

var validCategories = map[string]bool{
	"taches-repetitives": true,
	"outil-manquant":     true,
	"process-brouillon":  true,
	"intuition":          true,
}

func handleContact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	var req ContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON invalide", http.StatusBadRequest)
		return
	}

	// Validation
	if strings.TrimSpace(req.Journee) == "" {
		jsonError(w, "journee requis", http.StatusBadRequest)
		return
	}
	if !validCategories[req.Categorie] {
		jsonError(w, "categorie invalide", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Prenom) == "" {
		jsonError(w, "prenom requis", http.StatusBadRequest)
		return
	}
	if !isValidEmail(req.Email) {
		jsonError(w, "email invalide", http.StatusBadRequest)
		return
	}

	// Envoi des deux emails
	if err := sendNotification(req); err != nil {
		log.Printf("Erreur email notification: %v", err)
		http.Error(w, "Erreur envoi email", http.StatusInternalServerError)
		return
	}
	if err := sendConfirmation(req); err != nil {
		// Non bloquant — la notif est parties, on logue et on continue
		log.Printf("Erreur email confirmation: %v", err)
	}

	log.Printf("Nouveau contact: %s <%s> [%s]", req.Prenom, req.Email, req.Categorie)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func isValidEmail(email string) bool {
	email = strings.TrimSpace(email)
	if email == "" {
		return false
	}
	at := strings.LastIndex(email, "@")
	if at < 1 {
		return false
	}
	dot := strings.LastIndex(email[at:], ".")
	return dot > 1 && dot < len(email[at:])-1
}
