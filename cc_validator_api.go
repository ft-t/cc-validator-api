package cc_validator_api

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/tarm/serial"
)

const StartCode byte = 0x02
const PeripheralAddress byte = 0x03

type Baud int

const (
	Baud9600  Baud = 9600
	Baud19200 Baud = 19200
)

type Status byte

const (
	PowerUp                   Status = 0x10
	PowerUpWithBillValidator  Status = 0x11
	PowerUpWithBillStacker    Status = 0x12
	Initialize                Status = 0x13
	Idling                    Status = 0x14
	Accepting                 Status = 0x15
	Stacking                  Status = 0x17
	Returning                 Status = 0x18
	UnitDisabled              Status = 0x19
	Holding                   Status = 0x1A
	DeviceBusy                Status = 0x1B
	Rejecting                 Status = 0x1C
	DropCassetteFull          Status = 0x41
	DropCassetteOutOfPosition Status = 0x42
	ValidatorJammed           Status = 0x43
	DropCassetteJammed        Status = 0x44
	Cheated                   Status = 0x45
	GenericFailure            Status = 0x47
	EscrowPosition            Status = 0x80
	BillStacked               Status = 0x81
	BillReturned              Status = 0x82
)

//Rejecting Codes
const (
	DueToInsertion          byte = 0x60
	DueToMagnetic           byte = 0x61
	DueToRemainedBillInHead byte = 0x62
	DueToMultiplying        byte = 0x63
	DueToConveying          byte = 0x64
	DueToIdentification1    byte = 0x65
	DueToVerification       byte = 0x66
	DueToOptic              byte = 0x67
	DueToInhibit            byte = 0x68
	DueToCapacity           byte = 0x69
	DueToOperation          byte = 0x6A
	DueToLength             byte = 0x6C
)

//Failure Codes
const (
	StackMotorFailure            byte = 0x50
	TransportMotorSpeedFailure   byte = 0x51
	TransportMotorFailure        byte = 0x52
	AligningMotorFailure         byte = 0x53
	InitialCassetteStatusFailure byte = 0x54
	OpticCanalFailure            byte = 0x55
	MagneticCanalFailure         byte = 0x56
	CapacitanceCanalFailure      byte = 0x5F
)

type CCValidator struct {
	config *serial.Config
	port   *serial.Port
}

func NewConnection(path string, baud Baud) (CCValidator, error) {
	c := &serial.Config{Name: path, Baud: int(baud), ReadTimeout: 5 * time.Second} // TODO
	o, err := serial.OpenPort(c)

	res := CCValidator{}

	if err != nil {
		return res, err
	}

	res.config = c
	res.port = o

	//_,err = res.Reset()
	//
	//if err != nil {
	//	return res, err
	//}

	return res, nil
}

func (s *CCValidator) Reset() ([]byte, error) {
	sendRequest(s.port, 0x30, []byte{})
	return readResponse(s.port)
}

func (s *CCValidator) GetStatus() ([]byte, error) {
	sendRequest(s.port, 0x31, []byte{})
	return readResponse(s.port)
}

func (s *CCValidator) SetSecurity(data []byte) ([]byte, error) {
	sendRequest(s.port, 0x32, data)
	return readResponse(s.port)
}

func (s *CCValidator) Poll() (Status, byte, error) {
	sendRequest(s.port, 0x33, []byte{})
	response, err := readResponse(s.port)

	param := byte(0)
	if len(response) > 1 {
		param = response[1]
	}

	return Status(response[0]), param, err
}

func (s *CCValidator) Identification() ([]byte, error) {
	sendRequest(s.port, 0x37, []byte{})
	return readResponse(s.port)
}

func (s *CCValidator) GetBillTable() ([]byte, error) {
	sendRequest(s.port, 0x41, []byte{})
	return readResponse(s.port)
}

func (s *CCValidator) Ack() ([]byte, error) {
	sendRequest(s.port, 0x00, []byte{})
	return readResponse(s.port)
}

func Ack(port *serial.Port) {
	sendRequest(port, 0x00, []byte{})
}

func (s *CCValidator) Nack() ([]byte, error) {
	sendRequest(s.port, 0xFF, []byte{})
	return readResponse(s.port)
}

func readResponse(port *serial.Port) ([]byte, error) {
	var buf []byte
	innerBuf := make([]byte, 256)

	totalRead := 0
	readTriesCount := 0
	maxReadCount := 1050

	for ; ; {
		readTriesCount += 1

		if readTriesCount >= maxReadCount {
			return nil, fmt.Errorf("Reads tries exceeded")
		}

		n, err := port.Read(innerBuf)

		if err != nil {
			return nil, err
		}

		totalRead += n
		buf = append(buf, innerBuf[:n]...)

		if totalRead < 6 {
			continue
		}
		if buf[2] != 0x0 && int(buf[2]) != len(buf) {
			continue
		}

		break
	}

	if buf[0] != StartCode || buf[1] != PeripheralAddress {
		return nil, fmt.Errorf("Response format invalid")
	}

	crc := binary.LittleEndian.Uint16(buf[len(buf)-2:])

	buf = buf[:len(buf)-2]

	crc2 := GetCRC16(buf)

	if crc != crc2 {
		return nil, fmt.Errorf("Response verification failed")
	}

	if len(buf) == 4 && buf[3] == 0x00 {
		fmt.Printf("<- %X\n", buf)
		return nil, nil // TODO Ack
	}

	if len(buf) == 4 && buf[3] == 0xFF {
		return nil, fmt.Errorf("Nack")
	}

	if len(buf) == 4 && buf[3] == 0x30 {
		return nil, fmt.Errorf("Illegal command")
	}

	buf = buf[3:]

	fmt.Printf("<- %X\n", buf)
	Ack(port)

	return buf, nil
}

func sendRequest(port *serial.Port, commandCode byte, bytesData ...[]byte) {
	buf := new(bytes.Buffer)

	length := 6

	for _, b := range bytesData {
		length += len(b)
	}

	binary.Write(buf, binary.LittleEndian, StartCode)
	binary.Write(buf, binary.LittleEndian, PeripheralAddress)
	binary.Write(buf, binary.LittleEndian, byte(length))
	binary.Write(buf, binary.LittleEndian, commandCode)

	for _, data := range bytesData {
		binary.Write(buf, binary.LittleEndian, data)
	}

	crc := GetCRC16(buf.Bytes())

	binary.Write(buf, binary.LittleEndian, crc)
	fmt.Printf("-> %X\n", buf.Bytes())

	port.Write(buf.Bytes())
}

func GetCRC16(bufData []byte) uint16 {
	CRC := uint16(0)
	for i := 0; i < len(bufData); i++ {
		TmpCRC := CRC ^ uint16(bufData[i])
		for j := 0; j < 8; j++ {
			if (TmpCRC & 0x0001) > 0 {
				TmpCRC >>= 1
				TmpCRC ^= 0x08408
			} else {
				TmpCRC >>= 1
			}
		}
		CRC = TmpCRC
	}
	return CRC
}