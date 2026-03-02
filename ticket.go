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
	TICKET_TITLE_ID_OFFSET      = 468
	TICKET_TITLE_ID_SIZE        = 8
	TICKET_TITLE_VERSION_OFFSET = 486
	TICKET_TITLE_VERSION_SIZE   = 2
)

// Immutable default ticket payload; runtime fields are patched at known offsets.
const TICKET_TEMPLATE_HEX = "00010004d15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11ad15ea5ed15abe11a000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000526f6f742d434130303030303030332d585330303030303030630000000000000000000000000000000000000000000000000000000000000000000000000000feedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedface010000cccccccccccccccccccccccccccccccc00000000000000000000000000aaaaaaaaaaaaaaaa00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000010014000000ac000000140001001400000000000000280000000100000084000000840003000000000000ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

var (
	ticketTemplateOnce sync.Once
	ticketTemplateData []byte
	ticketTemplateErr  error
)

func GenerateTicket(path string, titleID uint64, titleKey []byte, titleVersion uint16) error {
	if len(titleKey) < TICKET_ENCRYPTED_KEY_SIZE {
		return fmt.Errorf("title key must be at least %d bytes, got %d", TICKET_ENCRYPTED_KEY_SIZE, len(titleKey))
	}

	var preservedEncryptedKey []byte
	var preservedKeyIndex byte
	hasPreservedKeyIndex := false
	if existingTicket, err := os.ReadFile(path); err == nil {
		if len(existingTicket) >= TICKET_ENCRYPTED_KEY_OFFSET+TICKET_ENCRYPTED_KEY_SIZE {
			preservedEncryptedKey = append([]byte(nil), existingTicket[TICKET_ENCRYPTED_KEY_OFFSET:TICKET_ENCRYPTED_KEY_OFFSET+TICKET_ENCRYPTED_KEY_SIZE]...)
		}
		if len(existingTicket) > TICKET_KEY_INDEX_OFFSET {
			preservedKeyIndex = existingTicket[TICKET_KEY_INDEX_OFFSET]
			hasPreservedKeyIndex = true
		}
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

	if len(preservedEncryptedKey) == TICKET_ENCRYPTED_KEY_SIZE {
		copy(ticketData[TICKET_ENCRYPTED_KEY_OFFSET:TICKET_ENCRYPTED_KEY_OFFSET+TICKET_ENCRYPTED_KEY_SIZE], preservedEncryptedKey)
	} else {
		copy(ticketData[TICKET_ENCRYPTED_KEY_OFFSET:TICKET_ENCRYPTED_KEY_OFFSET+TICKET_ENCRYPTED_KEY_SIZE], titleKey[:TICKET_ENCRYPTED_KEY_SIZE])
	}
	if hasPreservedKeyIndex {
		ticketData[TICKET_KEY_INDEX_OFFSET] = preservedKeyIndex
	}

	var titleIDBytes [TICKET_TITLE_ID_SIZE]byte
	binary.BigEndian.PutUint64(titleIDBytes[:], titleID)
	copy(ticketData[TICKET_TITLE_ID_OFFSET:TICKET_TITLE_ID_OFFSET+TICKET_TITLE_ID_SIZE], titleIDBytes[:])

	var versionBytes [TICKET_TITLE_VERSION_SIZE]byte
	binary.LittleEndian.PutUint16(versionBytes[:], titleVersion)
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

		requiredSize := maxInt(TICKET_ENCRYPTED_KEY_OFFSET+TICKET_ENCRYPTED_KEY_SIZE, TICKET_KEY_INDEX_OFFSET+1)
		requiredSize = maxInt(requiredSize, TICKET_TITLE_ID_OFFSET+TICKET_TITLE_ID_SIZE)
		requiredSize = maxInt(requiredSize, TICKET_TITLE_VERSION_OFFSET+TICKET_TITLE_VERSION_SIZE)
		if len(ticketTemplateData) < requiredSize {
			ticketTemplateErr = fmt.Errorf("ticket template too small: got %d, need at least %d", len(ticketTemplateData), requiredSize)
		}
	})
	if ticketTemplateErr != nil {
		return nil, ticketTemplateErr
	}
	return append([]byte(nil), ticketTemplateData...), nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
