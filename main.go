package main

import (
	"errors"
	"io"
	"log"
	"net/textproto"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/caarlos0/env/v6"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

type paychecks struct {
	Addr         string   `env:"IMAP_ADDR" envDefault:"imap.gmail.com:993"`
	Email        string   `env:"EMAIL"`
	Password     string   `env:"PASSWORD"`
	PDFPasswords []string `env:"PDF_PASSWORDS"`
	ID           string   `env:"ID"`
	OutputDir    string   `env:"OUTPUT_DIR" envDefault:"/tmp/output"`
	Filter       filter

	client *client.Client
}

type filter struct {
	Inbox   string   `env:"FILTER_INBOX" envDefault:"Inbox"`
	Subject string   `env:"FILTER_SUBJECT"`
	From    string   `env:"FILTER_FROM"`
	Body    []string `env:"FILTER_BODY" envSeparator:";" envDefault:"תלוש משכורת"`
}

var filePattern = regexp.MustCompile(`^\d{9}_20[1-2]\d_[0-1][0-9].pdf$`)

func allowedContentType(contentType string) bool {
	for _, c := range []string{"application/pdf", "application/octet-stream"} {
		if c == contentType {
			return true
		}
	}
	return false
}

func (p *paychecks) save(filename string, reader io.Reader) (string, error) {
	parts := strings.Split(filename, "_")
	outputDirPath := path.Join(p.OutputDir, parts[0], parts[1])
	outputPath := path.Join(outputDirPath, parts[2])
	if err := os.MkdirAll(outputDirPath, 0755); err != nil {
		return "", err
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		return "", err
	}

	return file.Name(), nil
}

func (p *paychecks) fetch(results []uint32, attachment func(filename string, reader io.Reader) error) error {
	seqSet := new(imap.SeqSet)
	seqSet.AddNum(results...)
	messages := make(chan *imap.Message, 100)
	section := &imap.BodySectionName{}
	_ = p.client.Fetch(seqSet, []imap.FetchItem{section.FetchItem(), imap.FetchEnvelope}, messages)

	for msg := range messages {
		log.Println("* " + msg.Envelope.Subject)
		r := msg.GetBody(section)

		mr, err := mail.CreateReader(r)
		if err != nil {
			return err
		}

		for {
			var part *mail.Part
			part, err = mr.NextPart()
			if err == io.EOF {
				break
			} else if err != nil {
				if errors.As(err, &message.UnknownCharsetError{}) {
					continue
				} else {
					return err
				}
			}

			switch h := part.Header.(type) {
			case *mail.AttachmentHeader:
				contentType, _, err := h.ContentType()
				if err != nil {
					return err
				}

				if !allowedContentType(contentType) {
					continue
				}

				filename, err := h.Filename()
				if err != nil {
					return err
				}

				if !filePattern.MatchString(filename) {
					log.Printf("file %q not match the filename pattern\n", filename)
					continue
				}

				log.Printf("handle attachment: %s\n", filename)
				if err = attachment(filename, part.Body); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func New() (*paychecks, error) {
	p := &paychecks{}
	if err := env.Parse(p); err != nil {
		return nil, err
	}
	return p, nil
}

func run() error {
	log.Println("Connecting to server...")

	p, err := New()
	if err != nil {
		return err
	}

	c, err := client.DialTLS(p.Addr, nil)
	if err != nil {
		return err
	}
	p.client = c
	log.Println("Connected")

	defer func() {
		_ = c.Logout()
	}()

	if err = c.Login(p.Email, p.Password); err != nil {
		return err
	}
	log.Println("Logged in")

	_, err = c.Select(p.Filter.Inbox, true)
	if err != nil {
		return err
	}
	sc := &imap.SearchCriteria{Body: p.Filter.Body, Header: textproto.MIMEHeader{}}
	if p.Filter.From != "" {
		sc.Header.Set("From", p.Filter.From)
	}
	if p.Filter.Subject != "" {
		sc.Header.Set("Subject", p.Filter.Subject)
	}

	searchResults, err := c.Search(sc)
	if err != nil {
		return err
	}
	log.Println("Search results: ", searchResults)

	err = p.fetch(searchResults, func(filename string, reader io.Reader) error {
		log.Printf("Found file %q", filename)
		savedFilename, err := p.save(filename, reader)
		if err != nil {
			return err
		}
		if err = decrypt(savedFilename, p.PDFPasswords); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.Println("Done!")

	return nil
}

func main() {
	err := run()
	if err != nil {
		log.Fatal("Failed to run: ", err)
	}
}
