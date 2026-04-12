package wiiudownloader

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
)

const (
	// Offsets below follow Nintendo's ticket binary layout.
	TICKET_ENCRYPTED_KEY_OFFSET = 0x1BF
	TICKET_ENCRYPTED_KEY_SIZE   = 0x10
	TICKET_KEY_INDEX_OFFSET     = 0x1F1
	TICKET_TITLE_ID_OFFSET      = 476
	TICKET_TITLE_ID_SIZE        = 8
	TICKET_TITLE_VERSION_OFFSET = 486
	TICKET_TITLE_VERSION_SIZE   = 2
)

// Immutable default ticket payload; runtime fields are patched at known offsets.
const TICKET_TEMPLATE_HEX = "" +
	"00010004d15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed" +
	"15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed" +
	"15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed" +
	"15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed" +
	"15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed" +
	"15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed" +
	"15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed" +
	"15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed" +
	"15abe11a00000000000000000000000000000000000000000000000000000000" +
	"0000000000000000000000000000000000000000000000000000000000000000" +
	"526f6f742d434130303030303030332d58533030303030303063000000000000" +
	"0000000000000000000000000000000000000000000000000000000000000000" +
	"feedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedface" +
	"feedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedface010000cc" +
	"cccccccccccccccccccccccccccccc00000000000000000000000000aaaaaaaa" +
	"aaaaaaaa00000000000000000000000000000000000000000000000000000000" +
	"0000000000000000000000000000000000000000000000000000000000000000" +
	"0001000000000000000000000000000000000000000000000000000000000000" +
	"0000000000000000000000000000000000000000000000000000000000000000" +
	"0000000000000000000000000000000000000000000000000000000000000000" +
	"0000000000000000000000000000000000000000000000000000000000000000" +
	"0000000000010014000000ac0000001400010014000000000000002800000001" +
	"00000084000000840003000000000000ffffffffffffffffffffffffffffffff" +
	"ffffffffffffffffffffffffffffffff00000000000000000000000000000000" +
	"0000000000000000000000000000000000000000000000000000000000000000" +
	"0000000000000000000000000000000000000000000000000000000000000000" +
	"00000000000000000000000000000000"

var (
	ticketTemplateOnce sync.Once
	ticketTemplateData []byte
	ticketTemplateErr  error
)

func GenerateTicket(path string, titleID uint64, titleKey []byte, titleVersion uint16) error {
	if len(titleKey) < TICKET_ENCRYPTED_KEY_SIZE {
		return fmt.Errorf("title key must be at least %d bytes, got %d", TICKET_ENCRYPTED_KEY_SIZE, len(titleKey))
	}

	ticketFile, err := os.Create(path)
	if err != nil {
		return err
	}
	defer ticketFile.Close()

	ticketData, err := newTicketData()
	if err != nil {
		return err
	}

	copy(ticketData[TICKET_ENCRYPTED_KEY_OFFSET:TICKET_ENCRYPTED_KEY_OFFSET+TICKET_ENCRYPTED_KEY_SIZE], titleKey[:TICKET_ENCRYPTED_KEY_SIZE])

	var titleIDBytes [TICKET_TITLE_ID_SIZE]byte
	binary.BigEndian.PutUint64(titleIDBytes[:], titleID)
	copy(ticketData[TICKET_TITLE_ID_OFFSET:TICKET_TITLE_ID_OFFSET+TICKET_TITLE_ID_SIZE], titleIDBytes[:])

	var versionBytes [TICKET_TITLE_VERSION_SIZE]byte
	binary.BigEndian.PutUint16(versionBytes[:], titleVersion)
	copy(ticketData[TICKET_TITLE_VERSION_OFFSET:TICKET_TITLE_VERSION_OFFSET+TICKET_TITLE_VERSION_SIZE], versionBytes[:])

	_, err = ticketFile.Write(ticketData)
	if err != nil {
		return err
	}

	return nil
}

func newTicketData() ([]byte, error) {
	ticketTemplateOnce.Do(func() {
		ticketTemplateData, ticketTemplateErr = hex.DecodeString(TICKET_TEMPLATE_HEX)
		if ticketTemplateErr != nil {
			return
		}

		requiredSize := TICKET_ENCRYPTED_KEY_OFFSET + TICKET_ENCRYPTED_KEY_SIZE
		if TICKET_KEY_INDEX_OFFSET+1 > requiredSize {
			requiredSize = TICKET_KEY_INDEX_OFFSET + 1
		}
		if TICKET_TITLE_ID_OFFSET+TICKET_TITLE_ID_SIZE > requiredSize {
			requiredSize = TICKET_TITLE_ID_OFFSET + TICKET_TITLE_ID_SIZE
		}
		if TICKET_TITLE_VERSION_OFFSET+TICKET_TITLE_VERSION_SIZE > requiredSize {
			requiredSize = TICKET_TITLE_VERSION_OFFSET + TICKET_TITLE_VERSION_SIZE
		}
		if len(ticketTemplateData) < requiredSize {
			ticketTemplateErr = fmt.Errorf("ticket template too small: got %d, need at least %d", len(ticketTemplateData), requiredSize)
		}
	})
	if ticketTemplateErr != nil {
		return nil, ticketTemplateErr
	}
	return append([]byte(nil), ticketTemplateData...), nil
}


