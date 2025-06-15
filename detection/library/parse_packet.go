package library

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"

	"github.com/pion/rtp"
)

type HTTPRequestData struct {
	Method        string
	Path          string
	ContentLength int64
	SrcID         uint16
	DstID         uint16
	PayloadType   uint16
	Timestamp     uint64
	Identifier    uint16
	Payload       []byte
}

type HTTPResponseData struct {
	Status        string
	StatusCode    int
	ContentLength int64
	SrcID         uint16
	DstID         uint16
	PayloadType   uint16
	Timestamp     uint64
	Identifier    uint16
	Payload       []byte
}

type RTPPacketData struct {
	Version        uint8
	PayloadType    uint8
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
	CSRC           []uint32
	Marker         bool
	Extension      bool
	ExtProfile     uint16
	SrcID          uint32
	DstID          uint32
	ExtTimestamp   uint64
	PayloadLength  uint32
	Payload        []byte
}

type RTPPacketData2 struct {
	Version        uint8
	PayloadType    uint8
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
	CSRC           []uint32
	Marker         bool
	Extension      bool
	ExtProfile     uint16
	ExtTimestamp   uint64
	Payload        []byte
}

func ParseHTTPRequest(req http.Request) (HTTPRequestData, error) {
	var data HTTPRequestData

	// Content-Length ヘッダからボディの長さを取得
	contentLength := req.ContentLength
	if contentLength < 0 && req.TransferEncoding == nil {
		return data, fmt.Errorf("invalid Content-Length and no Transfer-Encoding")
	}

	// ボディを読み込む
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return data, fmt.Errorf("failed to read request body: %w", err)
	}
	defer req.Body.Close()

	decodedBody, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return data, fmt.Errorf("failed to decode base64 body: %w", err)
	}
	bodyBuf := bytes.NewBuffer(decodedBody)
	// バッファを作成
	// bodyBuf := bytes.NewBuffer(body)

	// バイナリデータを解析
	endian := binary.BigEndian

	// 構造体にデータを格納
	data = HTTPRequestData{
		Method:        req.Method,
		Path:          req.URL.Path,
		ContentLength: contentLength,
	}

	// src_id (16ビット)
	if err := binary.Read(bodyBuf, endian, &data.SrcID); err != nil {
		return data, fmt.Errorf("failed to read src_id: %w", err)
	}

	// dst_id (16ビット)
	if err := binary.Read(bodyBuf, endian, &data.DstID); err != nil {
		return data, fmt.Errorf("failed to read dst_id: %w", err)
	}

	// payload_type (16ビット)
	if err := binary.Read(bodyBuf, endian, &data.PayloadType); err != nil {
		return data, fmt.Errorf("failed to read payload_type: %w", err)
	}

	// identifier (16ビット)
	if err := binary.Read(bodyBuf, endian, &data.Identifier); err != nil {
		return data, fmt.Errorf("failed to read identifier: %w", err)
	}

	// timestamp (64ビット)
	if err := binary.Read(bodyBuf, endian, &data.Timestamp); err != nil {
		return data, fmt.Errorf("failed to read timestamp: %w", err)
	}

	// 残りのデータを payload として読み取る
	data.Payload = bodyBuf.Bytes()

	return data, nil
}

func ParseHTTPResponse(resp http.Response) (HTTPResponseData, error) {
	var data HTTPResponseData

	// Content-Length ヘッダからボディの長さを取得
	contentLength := resp.ContentLength
	if contentLength <= 0 {
		return data, fmt.Errorf("invalid Content-Length: %d", contentLength)
	}

	// ボディを読み込む
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, fmt.Errorf("failed to read response body: %w", err)
	}

	decodedBody, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return data, fmt.Errorf("failed to decode base64 body: %w", err)
	}
	bodyBuf := bytes.NewBuffer(decodedBody)

	// バイナリデータを解析
	endian := binary.BigEndian

	// 構造体にデータを格納
	data = HTTPResponseData{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		ContentLength: contentLength,
	}

	// src_id (16ビット)
	if err := binary.Read(bodyBuf, endian, &data.SrcID); err != nil {
		return data, fmt.Errorf("failed to read src_id: %w", err)
	}

	// dst_id (16ビット)
	if err := binary.Read(bodyBuf, endian, &data.DstID); err != nil {
		return data, fmt.Errorf("failed to read dst_id: %w", err)
	}

	// payload_type (16ビット)
	if err := binary.Read(bodyBuf, endian, &data.PayloadType); err != nil {
		return data, fmt.Errorf("failed to read payload_type: %w", err)
	}

	// identifier (16ビット)
	if err := binary.Read(bodyBuf, endian, &data.Identifier); err != nil {
		return data, fmt.Errorf("failed to read identifier: %w", err)
	}

	// timestamp (64ビット)
	if err := binary.Read(bodyBuf, endian, &data.Timestamp); err != nil {
		return data, fmt.Errorf("failed to read timestamp: %w", err)
	}

	// 残りのデータを payload として読み取る
	data.Payload = bodyBuf.Bytes()

	return data, nil
}

func ParseRTPPacket(packet []byte) (RTPPacketData, error) {
	// RTPパケットをデコードするための変数
	var pkt rtp.Packet

	// まず固定ヘッダ部分をデコードする
	err := pkt.Unmarshal(packet)
	if err != nil {
		return RTPPacketData{}, fmt.Errorf("failed to unmarshal RTP packet: %v", err)
	}

	// 固定ヘッダの長さを計算（12バイト + CSRCの長さ）
	headerLen := 12 + len(pkt.Header.CSRC)*4

	// 構造体にヘッダ情報を格納
	rtpData := RTPPacketData{
		Version:        pkt.Header.Version,
		PayloadType:    pkt.Header.PayloadType,
		SequenceNumber: pkt.Header.SequenceNumber,
		Timestamp:      pkt.Header.Timestamp,
		SSRC:           pkt.Header.SSRC,
		CSRC:           pkt.Header.CSRC,
		Extension:      pkt.Header.Extension,
		Marker:         pkt.Header.Marker,
	}

	// 拡張ヘッダが存在する場合、手動で拡張ヘッダを解析
	if pkt.Header.Extension {
		// 拡張ヘッダの基本部分（4バイト）を取得
		extHeaderStart := headerLen
		extHeaderEnd := extHeaderStart + 4

		if extHeaderEnd > len(packet) {
			return RTPPacketData{}, fmt.Errorf("invalid extension header length")
		}

		extHeader := packet[extHeaderStart:extHeaderEnd]
		extProfile := binary.BigEndian.Uint16(extHeader[:2])
		extLength := binary.BigEndian.Uint16(extHeader[2:4]) * 4 // 長さは4バイト単位
		// fmt.Printf("ExtentionHeader Length : %d\n", extLength)
		rtpData.ExtProfile = extProfile

		// 拡張フィールドのデータ部分を取得
		extFieldsStart := extHeaderEnd
		extFieldsEnd := extFieldsStart + int(extLength)

		if extFieldsEnd > len(packet) {
			return RTPPacketData{}, fmt.Errorf("extension fields exceed packet length")
		}

		extData := packet[extFieldsStart:extFieldsEnd]
		extBuf := bytes.NewReader(extData)

		// 拡張フィールドを解析
		for extBuf.Len() > 0 {
			var idLen byte
			if err := binary.Read(extBuf, binary.BigEndian, &idLen); err != nil {
				return RTPPacketData{}, fmt.Errorf("failed to read extension field ID and length: %v", err)
			}

			id := idLen >> 4

			// 拡張フィールドのIDに基づいてデータ長とパディングを決定
			var dataLength int

			switch id {
			case 1, 2:
				dataLength = 4 // srcID, dstIDは16ビット（2バイト）
			case 3:
				dataLength = 4 // extTimestamp, payloadLengthは32ビット（4バイト）
			case 4:
				dataLength = 8
			default:
				return RTPPacketData{}, fmt.Errorf("unknown extension field ID: %d", id)
			}

			// データ部分を読み込む
			data := make([]byte, dataLength)
			if _, err := extBuf.Read(data); err != nil {
				return RTPPacketData{}, fmt.Errorf("failed to read extension field data: %v", err)
			}

			// 拡張フィールドのIDに基づいて構造体のフィールドに値を設定
			switch id {
			case 1:
				// srcIDは16ビットなので、uint32に変換
				rtpData.SrcID = uint32(binary.BigEndian.Uint32(data))
			case 2:
				// dstIDは16ビットなので、uint32に変換
				rtpData.DstID = uint32(binary.BigEndian.Uint32(data))
			case 3:
				rtpData.PayloadLength = uint32(binary.BigEndian.Uint32(data))
			case 4:
				rtpData.ExtTimestamp = uint64(binary.BigEndian.Uint64(data))
			}
		}

		// ペイロードの開始位置は拡張フィールドの終了位置
		payloadStart := extFieldsEnd
		if payloadStart > len(packet) {
			return RTPPacketData{}, fmt.Errorf("invalid payload start position")
		}

		rtpData.Payload = packet[payloadStart:]
	} else {
		// 拡張ヘッダがない場合、ペイロードの開始位置は固定ヘッダの終了位置
		payloadStart := headerLen
		rtpData.Payload = packet[payloadStart:]
	}

	return rtpData, nil
}

func ParseRTPPacket2(packet []byte) (RTPPacketData2, error) {
	// RTPパケットをデコードするための変数
	var pkt rtp.Packet

	// まず固定ヘッダ部分をデコードする
	err := pkt.Unmarshal(packet)
	if err != nil {
		return RTPPacketData2{}, fmt.Errorf("failed to unmarshal RTP packet: %v", err)
	}

	// 固定ヘッダの長さを計算（12バイト + CSRCの長さ）
	headerLen := 12 + len(pkt.Header.CSRC)*4

	// 構造体にヘッダ情報を格納
	rtpData := RTPPacketData2{
		Version:        pkt.Header.Version,
		PayloadType:    pkt.Header.PayloadType,
		SequenceNumber: pkt.Header.SequenceNumber,
		Timestamp:      pkt.Header.Timestamp,
		SSRC:           pkt.Header.SSRC,
		CSRC:           pkt.Header.CSRC,
		Extension:      pkt.Header.Extension,
		Marker:         pkt.Header.Marker,
	}

	// 拡張ヘッダが存在する場合、手動で拡張ヘッダを解析
	if pkt.Header.Extension {
		// 拡張ヘッダの基本部分（4バイト）を取得
		extHeaderStart := headerLen
		extHeaderEnd := extHeaderStart + 4

		if extHeaderEnd > len(packet) {
			return RTPPacketData2{}, fmt.Errorf("invalid extension header length")
		}

		extHeader := packet[extHeaderStart:extHeaderEnd]
		extProfile := binary.BigEndian.Uint16(extHeader[:2])
		extLength := binary.BigEndian.Uint16(extHeader[2:4]) * 4 // 長さは4バイト単位
		// fmt.Printf("ExtentionHeader Length : %d\n", extLength)
		rtpData.ExtProfile = extProfile

		// 拡張フィールドのデータ部分を取得
		extFieldsStart := extHeaderEnd
		extFieldsEnd := extFieldsStart + int(extLength)

		if extFieldsEnd > len(packet) {
			return RTPPacketData2{}, fmt.Errorf("extension fields exceed packet length")
		}

		// extData := packet[extFieldsStart:extFieldsEnd]
		// extBuf := bytes.NewReader(extData)

		actual := pkt.GetExtension(1)
		rtpData.ExtTimestamp = uint64(binary.BigEndian.Uint64(actual))
		// // 拡張フィールドを解析
		// for extBuf.Len() > 0 {
		// 	var idLen byte
		// 	if err := binary.Read(extBuf, binary.BigEndian, &idLen); err != nil {
		// 		return RTPPacketData2{}, fmt.Errorf("failed to read extension field ID and length: %v", err)
		// 	}

		// 	id := idLen >> 4

		// 	if id == 0 {
		// 		// これはパディング → データ長０、ただスキップ
		// 		continue
		// 	}

		// 	// 拡張フィールドのIDに基づいてデータ長とパディングを決定
		// 	var dataLength int

		// 	switch id {
		// 	case 1:
		// 		dataLength = 8 // srcID, dstIDは16ビット（2バイト）
		// 	default:
		// 		return RTPPacketData2{}, fmt.Errorf("unknown extension field ID: %d", id)
		// 	}

		// 	// データ部分を読み込む
		// 	data := make([]byte, dataLength)
		// 	if _, err := extBuf.Read(data); err != nil {
		// 		return RTPPacketData2{}, fmt.Errorf("failed to read extension field data: %v", err)
		// 	}

		// 	// 拡張フィールドのIDに基づいて構造体のフィールドに値を設定
		// 	switch id {
		// 	case 1:
		// 		// srcIDは16ビットなので、uint32に変換
		// 		rtpData.ExtTimestamp = uint64(binary.BigEndian.Uint64(data))
		// 	}
		// }

		// ペイロードの開始位置は拡張フィールドの終了位置
		payloadStart := extFieldsEnd
		if payloadStart > len(packet) {
			return RTPPacketData2{}, fmt.Errorf("invalid payload start position")
		}

		rtpData.Payload = packet[payloadStart:]
	} else {
		// 拡張ヘッダがない場合、ペイロードの開始位置は固定ヘッダの終了位置
		payloadStart := headerLen
		rtpData.Payload = packet[payloadStart:]
	}

	return rtpData, nil
}
