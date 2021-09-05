# Paychecks
Simple utility that fetches encrypted paychecks from your Inbox decrypt and organize them for easy-access.

## Usage
First, create `.env` file with your configuration or just edit the environment on `docker-compose.yml`

| Field      | Description |
| ----------- | ----------- |
| EMAIL      | The Email to search the files within |
| PASSWORD   | Password used to access to Email. If you use Gmail you can generate App Password |
| ID   | Your ID. The paychecks file names are expected to be in the following format: `<ID>_<YEAY>_<MONTH>.pdf`|
| PDF_PASSWORDS   | Comma separated list of passwords to use for decrypting the files |
| IMAP_ADDR | Optional. IMAP address to connect, default: `imap.gmail.com:993`
| FILTER_INBOX   | Optional. The inbox to search the mails within, default: "Inbox" |
| FILTER_BODY   | Optional. List of expression to lookup for in mail body separated by `;`, default: "תלוש משכורת" |
| FILTER_FROM   | Optional. Filter only mails that received from the given sender |

Now, you can simply run:
```
docker-compose up --remove-orphans
```