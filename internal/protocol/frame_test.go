package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeFrame(t *testing.T) {
	tests := []struct {
		name      string
		channelID uint16
		signal    uint16
		data      []byte
		want      []byte
	}{
		{
			name:      "空数据帧",
			channelID: 0,
			signal:    SIG_HEARTBEAT,
			data:      nil,
			want:      []byte{0x00, 0x00, 0x00, 0x06},
		},
		{
			name:      "注册消息",
			channelID: 0,
			signal:    SIG_REGISTER,
			data:      []byte("user|host|model|arch|22"),
			want:      append([]byte{0x00, 0x00, 0x00, 0x01}, []byte("user|host|model|arch|22")...),
		},
		{
			name:      "通道数据",
			channelID: 1,
			signal:    SIG_CHANNEL_DATA,
			data:      []byte("test data"),
			want:      append([]byte{0x00, 0x01, 0x00, 0x04}, []byte("test data")...),
		},
		{
			name:      "大通道ID",
			channelID: 65535,
			signal:    SIG_CHANNEL_CLOSE,
			data:      nil,
			want:      []byte{0xFF, 0xFF, 0x00, 0x05},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeFrame(tt.channelID, tt.signal, tt.data)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("EncodeFrame() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeFrame(t *testing.T) {
	tests := []struct {
		name          string
		frame         []byte
		wantChannelID uint16
		wantSignal    uint16
		wantData      []byte
		wantErr       error
	}{
		{
			name:          "空数据帧",
			frame:         []byte{0x00, 0x00, 0x00, 0x06},
			wantChannelID: 0,
			wantSignal:    SIG_HEARTBEAT,
			wantData:      nil,
			wantErr:       nil,
		},
		{
			name:          "注册消息",
			frame:         append([]byte{0x00, 0x00, 0x00, 0x01}, []byte("user|host|model|arch|22")...),
			wantChannelID: 0,
			wantSignal:    SIG_REGISTER,
			wantData:      []byte("user|host|model|arch|22"),
			wantErr:       nil,
		},
		{
			name:          "通道数据",
			frame:         append([]byte{0x00, 0x01, 0x00, 0x04}, []byte("test data")...),
			wantChannelID: 1,
			wantSignal:    SIG_CHANNEL_DATA,
			wantData:      []byte("test data"),
			wantErr:       nil,
		},
		{
			name:          "帧太短",
			frame:         []byte{0x00, 0x01},
			wantChannelID: 0,
			wantSignal:    0,
			wantData:      nil,
			wantErr:       ErrFrameTooShort,
		},
		{
			name:          "空帧",
			frame:         []byte{},
			wantChannelID: 0,
			wantSignal:    0,
			wantData:      nil,
			wantErr:       ErrFrameTooShort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotChannelID, gotSignal, gotData, gotErr := DecodeFrame(tt.frame)

			if gotErr != tt.wantErr {
				t.Errorf("DecodeFrame() error = %v, wantErr %v", gotErr, tt.wantErr)
				return
			}

			if gotChannelID != tt.wantChannelID {
				t.Errorf("DecodeFrame() channelID = %v, want %v", gotChannelID, tt.wantChannelID)
			}

			if gotSignal != tt.wantSignal {
				t.Errorf("DecodeFrame() signal = %v, want %v", gotSignal, tt.wantSignal)
			}

			if !bytes.Equal(gotData, tt.wantData) {
				t.Errorf("DecodeFrame() data = %v, want %v", gotData, tt.wantData)
			}
		})
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		channelID uint16
		signal    uint16
		data      []byte
	}{
		{
			name:      "心跳",
			channelID: 0,
			signal:    SIG_HEARTBEAT,
			data:      nil,
		},
		{
			name:      "注册",
			channelID: 0,
			signal:    SIG_REGISTER,
			data:      []byte("user|host|model|arch|22"),
		},
		{
			name:      "通道数据",
			channelID: 123,
			signal:    SIG_CHANNEL_DATA,
			data:      []byte("some SSH data here"),
		},
		{
			name:      "大数据",
			channelID: 1,
			signal:    SIG_CHANNEL_DATA,
			data:      make([]byte, 65536), // 64KB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 编码
			encoded := EncodeFrame(tt.channelID, tt.signal, tt.data)

			// 解码
			gotChannelID, gotSignal, gotData, err := DecodeFrame(encoded)
			if err != nil {
				t.Fatalf("DecodeFrame() error = %v", err)
			}

			// 验证
			if gotChannelID != tt.channelID {
				t.Errorf("channelID = %v, want %v", gotChannelID, tt.channelID)
			}

			if gotSignal != tt.signal {
				t.Errorf("signal = %v, want %v", gotSignal, tt.signal)
			}

			if !bytes.Equal(gotData, tt.data) {
				t.Errorf("data length = %v, want %v", len(gotData), len(tt.data))
			}
		})
	}
}

func BenchmarkEncodeFrame(b *testing.B) {
	data := make([]byte, 1024)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		EncodeFrame(1, SIG_CHANNEL_DATA, data)
	}
}

func BenchmarkDecodeFrame(b *testing.B) {
	data := make([]byte, 1024)
	frame := EncodeFrame(1, SIG_CHANNEL_DATA, data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		DecodeFrame(frame)
	}
}
