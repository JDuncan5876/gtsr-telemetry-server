package listener

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"server/configs"
	"server/datatypes"
)

const (
	idle          ReceiverState = 0
	pre1          ReceiverState = 1
	pre2          ReceiverState = 2
	pre3          ReceiverState = 3
	preambleRecvd ReceiverState = 4
)

// ReceiverState is the enumerated type for receiver state
type ReceiverState int

// PacketParser is a interface describe an object which parses incoming packets from the car
type PacketParser interface {
	ParseByte(value byte) bool
	ParsePacket() []*datatypes.Datapoint
}

// NewPacketParser returns a new PacketParser with the standard implementation
func NewPacketParser(canConfigs map[int][]*configs.CanConfigType) PacketParser {
	return &packetParser{
		State:        idle,
		PacketBuffer: make([]byte, 16),
		CANConfigs:   canConfigs,
	}
}

type packetParser struct {
	State        ReceiverState
	PacketBuffer []byte
	Offset       int
	CANConfigs   map[int][]*configs.CanConfigType
}

// PayloadParsers maps datatype strings to methods to parse a value in bytes at a given offset
var PayloadParsers map[string]func([]byte, int) (float64, error)

// ParseByte maintains the parser state machine, parsing one byte at a time
// It returns true when the full packet has been received
func (p *packetParser) ParseByte(value byte) bool {
	switch p.State {
	case idle:
		if value == byte('G') {
			p.State = pre1
		} else {
			p.State = idle
		}
	case pre1:
		if value == byte('T') {
			p.State = pre2
		} else {
			p.State = idle
		}
	case pre2:
		if value == byte('S') {
			p.State = pre3
		} else {
			p.State = idle
		}
	case pre3:
		if value == byte('R') {
			p.State = preambleRecvd
			p.Offset = 4 // Preamble offset
		} else {
			p.State = idle
		}
	case preambleRecvd:
		p.PacketBuffer[p.Offset] = value
		p.Offset++
		if p.Offset >= len(p.PacketBuffer) {
			p.State = idle
			return true
		}
	default:
		fmt.Println("Unrecognized packet parser state: ", p.State)
		p.State = idle
	}
	return false
}

// ParsePacket returns the datapoint parsed from the current packet saved within the parser
func (p *packetParser) ParsePacket() []*datatypes.Datapoint {
	canID := int(binary.LittleEndian.Uint16(p.PacketBuffer[4:6]))
	canConfigs := p.CANConfigs[canID]
	points := make([]*datatypes.Datapoint, 0)
	for _, config := range canConfigs {
		point := &datatypes.Datapoint{
			Metric: config.Name,
			Time:   time.Now(),
		}
		converter, ok := PayloadParsers[config.Datatype]
		if !ok {
			fmt.Println("Unrecognized datatype: " + config.Datatype)
			continue
		}
		value, err := converter(p.PacketBuffer[8:], config.Offset)
		if err != nil {
			fmt.Printf("Error parsing %s: %s\n", config.Datatype, err)
			continue
		}
		point.Value = value
		if !config.CheckBounds || (config.MinValue <= point.Value && config.MaxValue >= point.Value) {
			points = append(points, point)
		}
	}
	return points
}

func init() {
	PayloadParsers = make(map[string]func([]byte, int) (float64, error))
	PayloadParsers["uint8"] = func(bytes []byte, offset int) (float64, error) {
		return float64(bytes[offset]), nil
	}
	PayloadParsers["uint16"] = func(bytes []byte, offset int) (float64, error) {
		return float64(binary.LittleEndian.Uint16(bytes[offset : offset+2])), nil
	}
	PayloadParsers["uint32"] = func(bytes []byte, offset int) (float64, error) {
		return float64(binary.LittleEndian.Uint32(bytes[offset : offset+4])), nil
	}
	PayloadParsers["uint64"] = func(bytes []byte, offset int) (float64, error) {
		return float64(binary.LittleEndian.Uint64(bytes[offset : offset+8])), nil
	}
	PayloadParsers["int8"] = func(bytes []byte, offset int) (float64, error) {
		return float64(int8(bytes[offset])), nil
	}
	PayloadParsers["int16"] = func(bytes []byte, offset int) (float64, error) {
		return float64(int16(binary.LittleEndian.Uint16(bytes[offset : offset+2]))), nil
	}
	PayloadParsers["int32"] = func(bytes []byte, offset int) (float64, error) {
		return float64(int32(binary.LittleEndian.Uint32(bytes[offset : offset+4]))), nil
	}
	PayloadParsers["int64"] = func(bytes []byte, offset int) (float64, error) {
		return float64(int64(binary.LittleEndian.Uint64(bytes[offset : offset+8]))), nil
	}
	PayloadParsers["float32"] = func(bytes []byte, offset int) (float64, error) {
		rawValue := binary.LittleEndian.Uint32(bytes[offset : offset+4])
		value := float64(math.Float32frombits(rawValue))
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return value, fmt.Errorf("invalid float value parsed from packet")
		}
		return value, nil
	}
	PayloadParsers["float64"] = func(bytes []byte, offset int) (float64, error) {
		rawValue := binary.LittleEndian.Uint64(bytes[offset : offset+8])
		value := math.Float64frombits(rawValue)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return value, fmt.Errorf("invalid float value parsed from packet")
		}
		return value, nil
	}
}