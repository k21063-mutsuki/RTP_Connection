// ライブラリ機能
package library

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pion/rtp"
)

// CreateHTTPRequest はHTTPリクエストを生成し、ソケット経由で送信する
func CreateHTTPRequest(
	// conn net.Conn,
	host string,
	method string,
	url *url.URL,
	src_id uint16,
	dst_id uint16,
	payload_type uint16,
	identifier uint16,
	payload []byte,
) (string, http.Request, HTTPRequestData, error) {
	// ボディ構築用のバッファ
	var bodyBuf bytes.Buffer

	// バイナリ書き込み時のエンディアンをBigEndianに指定
	endian := binary.BigEndian

	// src_id (16ビット)
	if err := binary.Write(&bodyBuf, endian, uint16(src_id)); err != nil {
		return "", http.Request{}, HTTPRequestData{}, fmt.Errorf("failed to write src_id: %w", err)
	}

	// dst_id (16ビット)
	if err := binary.Write(&bodyBuf, endian, uint16(dst_id)); err != nil {
		return "", http.Request{}, HTTPRequestData{}, fmt.Errorf("failed to write dst_id: %w", err)
	}

	// payload_type (8ビット)
	if err := binary.Write(&bodyBuf, endian, uint16(payload_type)); err != nil {
		return "", http.Request{}, HTTPRequestData{}, fmt.Errorf("failed to write payload_type: %w", err)
	}

	// identifier (16ビット)
	if err := binary.Write(&bodyBuf, endian, uint16(identifier)); err != nil {
		return "", http.Request{}, HTTPRequestData{}, fmt.Errorf("failed to write identifier: %w", err)
	}

	// times, err := ntp.Time("time.google.com")
	// if err != nil {
	// 	return "", http.Request{}, fmt.Errorf("failed to create timestamp: %w", err), RequestData{}
	// }

	times := time.Now()
	// timestamp (64ビット)
	if err := binary.Write(&bodyBuf, endian, uint64(times.UnixNano())); err != nil {
		return "", http.Request{}, HTTPRequestData{}, fmt.Errorf("failed to write timestamp: %w", err)
	}

	// payload (可変長)
	if _, err := bodyBuf.Write(payload); err != nil {
		return "", http.Request{}, HTTPRequestData{}, fmt.Errorf("failed to write payload: %w", err)
	}

	// fmt.Println(bodyBuf)
	encodedBody := base64.StdEncoding.EncodeToString(bodyBuf.Bytes())

	// fmt.Println("body_length: %d", len(encodedBody))

	body := io.NopCloser(strings.NewReader(encodedBody))

	// body := io.NopCloser(&bodyBuf)

	req := http.Request{
		Method:        method,                  // HTTPメソッド (GET, POSTなど)
		URL:           url,                     // リクエスト先のURL (net/url.URL型)
		Proto:         "HTTP/1.1",              // 使用するHTTPプロトコルのバージョン
		ProtoMajor:    1,                       // HTTPのメジャーバージョン
		ProtoMinor:    1,                       // HTTPのマイナーバージョン
		Header:        make(http.Header),       // リクエストヘッダ
		Body:          body,                    // リクエストボディ (io.ReadCloser型)
		ContentLength: int64(len(encodedBody)), // ボディの長さ
		Host:          host,                    // ホスト名
	}

	// ヘッダ設定
	req.Header.Set("Content-Type", "application/octet-stream")

	data := HTTPRequestData{
		Method:        method,
		Path:          url.Path,
		ContentLength: int64(len(encodedBody)),
		SrcID:         src_id,
		DstID:         dst_id,
		PayloadType:   payload_type,
		Timestamp:     uint64(times.UnixNano()),
		Identifier:    identifier,
		Payload:       payload,
	}

	return "success", req, data, nil
}

func CreateHTTPResponse(
	// conn net.Conn,
	status string,
	statusCode uint16,
	srcID uint16,
	dstID uint16,
	payloadType uint16,
	identifier uint16,
	payload []byte,
) (string, http.Response, HTTPResponseData, error) {
	// ボディ構築用のバッファ
	var bodyBuf bytes.Buffer

	// バイナリ書き込み時のエンディアンをBigEndianに指定
	endian := binary.BigEndian

	// srcID (16ビット)
	if err := binary.Write(&bodyBuf, endian, uint16(srcID)); err != nil {
		return "", http.Response{}, HTTPResponseData{}, fmt.Errorf("failed to write src_id: %w", err)
	}

	// dstID (16ビット)
	if err := binary.Write(&bodyBuf, endian, uint16(dstID)); err != nil {
		return "", http.Response{}, HTTPResponseData{}, fmt.Errorf("failed to write dst_id: %w", err)
	}

	// payloadType (8ビット)
	if err := binary.Write(&bodyBuf, endian, uint16(payloadType)); err != nil {
		return "", http.Response{}, HTTPResponseData{}, fmt.Errorf("failed to write payloadtype: %w", err)
	}

	// identifier (16ビット)
	if err := binary.Write(&bodyBuf, endian, uint16(identifier)); err != nil {
		return "", http.Response{}, HTTPResponseData{}, fmt.Errorf("failed to write identifier: %w", err)
	}

	// times, err := ntp.Time("time.google.com")
	// if err != nil {
	// 	return "", http.Response{}, fmt.Errorf("failed to create timestamp: %w", err), ResponseData{}
	// }
	times := time.Now()
	// timestamp (64ビット)
	if err := binary.Write(&bodyBuf, endian, uint64(times.UnixNano())); err != nil {
		return "", http.Response{}, HTTPResponseData{}, fmt.Errorf("failed to write timestamp: %w", err)
	}

	// payload (可変長)
	if _, err := bodyBuf.Write(payload); err != nil {
		return "", http.Response{}, HTTPResponseData{}, fmt.Errorf("failed to write payload: %w", err)
	}

	encodedBody := base64.StdEncoding.EncodeToString(bodyBuf.Bytes())

	body := io.NopCloser(strings.NewReader(encodedBody))
	// body := io.NopCloser(&bodyBuf)

	// HTTPレスポンスの生成
	resp := http.Response{
		Status:        status,
		StatusCode:    int(statusCode),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          body,
		ContentLength: int64(len(encodedBody)),
	}

	// レスポンスヘッダの設定
	resp.Header.Set("Content-Type", "application/octet-stream")

	data := HTTPResponseData{
		Status:        status,
		StatusCode:    int(statusCode),
		ContentLength: int64(len(encodedBody)),
		SrcID:         srcID,
		DstID:         dstID,
		PayloadType:   payloadType,
		Timestamp:     uint64(times.UnixNano()),
		Identifier:    identifier,
		Payload:       payload,
	}

	return "success", resp, data, nil
}

func CreateRTPPacket(
	// conn net.Conn,
	payloadType uint8,
	sequenceNumber uint16,
	timestamp uint32,
	ssrc uint32,
	csrcList []uint32,
	srcID uint32,
	dstID uint32,
	binaryData []byte, // 画像などのバイナリデータを受け取る
	marker bool, // Markerビットの追加
) (string, []byte, RTPPacketData, error) {
	// RTPパケットの作成
	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:          2,
			PayloadType:      uint8(payloadType),
			SequenceNumber:   uint16(sequenceNumber),
			Timestamp:        uint32(timestamp),
			SSRC:             uint32(ssrc),
			CSRC:             []uint32(csrcList),
			Marker:           marker, // Markerビットをセット
			Extension:        true,
			ExtensionProfile: 0xBEDE,
			Extensions:       []rtp.Extension{},
		},
		Payload: binaryData,
	}

	srcid := make([]byte, 4)
	binary.BigEndian.PutUint32(srcid, srcID)
	pkt.SetExtension(1, srcid)

	dstid := make([]byte, 4)
	binary.BigEndian.PutUint32(dstid, dstID)
	pkt.SetExtension(2, dstid)

	payloadlength := make([]byte, 4)
	length := len(binaryData)
	binary.BigEndian.PutUint32(payloadlength, uint32(length))
	pkt.SetExtension(3, payloadlength)

	extimestamp := make([]byte, 8)
	times := time.Now()
	binary.BigEndian.PutUint64(extimestamp, uint64(times.UnixNano()))
	pkt.SetExtension(4, extimestamp)

	raw, err := pkt.Marshal()
	if err != nil {
		return "", nil, RTPPacketData{}, fmt.Errorf("error marshaling RTP packet: %w", err)
	}

	data := RTPPacketData{
		PayloadType:    payloadType,
		SequenceNumber: sequenceNumber,
		Timestamp:      timestamp,
		SSRC:           ssrc,
		CSRC:           []uint32(csrcList),
		Marker:         marker,
		Extension:      true,
		SrcID:          uint32(srcID),
		DstID:          uint32(dstID),
		ExtTimestamp:   uint64(times.UnixNano()),
		PayloadLength:  uint32(len(binaryData)),
		Payload:        binaryData,
	}

	return "success", raw, data, nil
}

func CreateRTPPacket2(
	// conn net.Conn,
	payloadType uint8,
	sequenceNumber uint16,
	timestamp uint32,
	ssrc uint32,
	csrcList []uint32,
	binaryData []byte, // 画像などのバイナリデータを受け取る
	marker bool, // Markerビットの追加
) (string, []byte, RTPPacketData2, error) {
	// RTPパケットの作成
	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:          2,
			PayloadType:      uint8(payloadType),
			SequenceNumber:   uint16(sequenceNumber),
			Timestamp:        uint32(timestamp),
			SSRC:             uint32(ssrc),
			CSRC:             []uint32(csrcList),
			Marker:           marker, // Markerビットをセット
			Extension:        true,
			ExtensionProfile: 0xBEDE,
			Extensions:       []rtp.Extension{},
		},
		Payload: binaryData,
	}

	extimestamp := make([]byte, 8)
	times := time.Now()
	binary.BigEndian.PutUint64(extimestamp, uint64(times.UnixNano()))
	pkt.SetExtension(1, extimestamp)

	raw, err := pkt.Marshal()
	if err != nil {
		return "", nil, RTPPacketData2{}, fmt.Errorf("error marshaling RTP packet: %w", err)
	}

	data := RTPPacketData2{
		PayloadType:    payloadType,
		SequenceNumber: sequenceNumber,
		Timestamp:      timestamp,
		SSRC:           ssrc,
		CSRC:           []uint32(csrcList),
		Marker:         marker,
		Extension:      true,
		ExtTimestamp:   uint64(times.UnixNano()),
		Payload:        binaryData,
	}

	return "success", raw, data, nil
}
