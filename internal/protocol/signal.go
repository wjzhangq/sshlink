package protocol

// 控制信号常量定义
const (
	// SIG_REGISTER 客户端注册自身信息
	// 数据格式: 用户名|主机名|电脑型号|CPU架构|SSH端口
	SIG_REGISTER uint16 = 0x0001

	// SIG_REGISTER_ACK 服务端确认注册，返回客户端ID和公钥
	// 数据格式: 客户端ID\n公钥内容
	SIG_REGISTER_ACK uint16 = 0x0002

	// SIG_NEW_CHANNEL 服务端请求创建新的SSH通道
	// 无数据，仅头部
	SIG_NEW_CHANNEL uint16 = 0x0003

	// SIG_CHANNEL_DATA 通道数据传输
	// 数据格式: SSH原始字节流
	SIG_CHANNEL_DATA uint16 = 0x0004

	// SIG_CHANNEL_CLOSE 关闭指定通道
	// 无数据
	SIG_CHANNEL_CLOSE uint16 = 0x0005

	// SIG_HEARTBEAT 心跳检测
	// 无数据
	SIG_HEARTBEAT uint16 = 0x0006
)
