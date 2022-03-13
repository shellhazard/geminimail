package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid"
	"github.com/shellhazard/tmm"

	"git.sr.ht/~adnano/go-gemini"
	"git.sr.ht/~adnano/go-gemini/certificate"
)

type SessionManager struct {
	store map[string]*tmm.Session

	mu sync.Mutex
}

func (s *SessionManager) Get(key string) (*tmm.Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.store[key]

	return sess, ok
}

func (s *SessionManager) Set(key string, sess *tmm.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.store[key] = sess
}

func (s *SessionManager) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.store, key)
}

var sessions = &SessionManager{
	store: make(map[string]*tmm.Session),
}

func main() {
	if len(os.Args) < 2 {
		log.Printf("usage: %s [hostname]\n", os.Args[0])
		os.Exit(1)
	}

	// Create a certificate store, register the
	// "localhost" self-signed certificate.
	certificates := &certificate.Store{}
	certificates.Register(os.Args[1])
	if err := certificates.Load("./"); err != nil {
		log.Fatal(err)
	}

	// Prepare mux
	mux := &gemini.Mux{}
	mux.Handle("/", gemini.FileServer(os.DirFS("./static")))
	mux.Handle("/new", gemini.HandlerFunc(func(ctx context.Context, rw gemini.ResponseWriter, r *gemini.Request) {
		// Generate new token
		uuid, err := uuid.NewV4()
		if err != nil {
			log.Printf("error: %s", err)
			rw.WriteHeader(gemini.StatusCGIError, "Unexpected error")
		}

		// Create new disposable mail
		s, err := tmm.New()
		if err != nil {
			log.Printf("error: %s", err)
			rw.WriteHeader(gemini.StatusCGIError, "Unexpected error")
		}

		// Store session
		sessions.Set(uuid.String(), s)

		// Redirect to the mail page
		rw.WriteHeader(gemini.StatusRedirect, fmt.Sprintf("/mail?t=%s", uuid.String()))
	}))

	mux.Handle("/mail", gemini.HandlerFunc(func(ctx context.Context, rw gemini.ResponseWriter, r *gemini.Request) {
		values := r.URL.Query()
		tokens, ok := values["t"]
		if !ok {
			rw.WriteHeader(gemini.StatusRedirect, "/")
			return
		}

		// No token provided, redirect to homepage
		if len(tokens) < 1 {
			rw.WriteHeader(gemini.StatusRedirect, "/")
			return
		}

		// Check if the token exists
		s, ok := sessions.Get(tokens[0])
		if !ok {
			rw.WriteHeader(gemini.StatusRedirect, "/")
			return
		}

		// Check if the token has expired
		if s.Expired() {
			rw.WriteHeader(gemini.StatusRedirect, "/")
			sessions.Delete(tokens[0])
			return
		}

		// Prepare page
		addr := s.Address()
		mail, err := s.Messages()
		if err != nil {
			rw.WriteHeader(gemini.StatusRedirect, "/")
			sessions.Delete(tokens[0])
			return
		}

		// Renew the address
		ok, err = s.Renew()
		if !ok || err != nil {
			rw.WriteHeader(gemini.StatusRedirect, "/")
			sessions.Delete(tokens[0])
			return
		}

		output := []string{}
		for _, m := range mail {
			// Prepare console output
			var out strings.Builder
			out.WriteString(fmt.Sprintf("Sender: %s\n", m.Sender))
			out.WriteString(fmt.Sprintf("At: %s\n", m.SentDate.UTC().Format(time.RFC822)))
			out.WriteString(fmt.Sprintf("Subject: %s\n", m.Subject))
			out.WriteString(fmt.Sprintf("Body: %s\n", strings.TrimSpace(m.Plaintext)))

			output = append(output, out.String())
		}

		fmt.Fprint(rw, "# Mailbox\n\n")
		fmt.Fprintf(rw, "Your address is: %s.\n\n", addr)
		fmt.Fprintf(rw, "It will expire on %s.\n\n", s.ExpiresAt().UTC().Format(time.RFC822))

		fmt.Fprint(rw, "## Messages\n\n")
		if len(output) > 0 {
			fmt.Fprint(rw, "```\n")
			fmt.Fprint(rw, strings.Join(output, "-----\n"))
			fmt.Fprint(rw, "```\n")
		} else {
			fmt.Fprint(rw, "Nothing here yet.\n\n")
		}
		fmt.Fprintf(rw, "=> mail?t=%s Reload this page", tokens[0])
	}))

	server := &gemini.Server{
		Handler:        gemini.LoggingMiddleware(mux),
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   1 * time.Minute,
		GetCertificate: certificates.Get,
	}

	// Listen for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	errch := make(chan error)
	go func() {
		ctx := context.Background()
		errch <- server.ListenAndServe(ctx)
	}()

	select {
	case err := <-errch:
		log.Fatal(err)
	case <-c:
		// Shutdown the server
		log.Println("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := server.Shutdown(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}
}
