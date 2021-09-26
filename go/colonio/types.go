package colonio

// Colonio is an interface. It is equivalent to one node.
type Colonio interface {
	Connect(url, token string) error
	Disconnect() error
	AccessMap(name string) Map
	AccessPubsub2D(name string) Pubsub2D
	GetLocalNid() string
	SetPosition(x, y float64) (float64, float64, error)
	Quit() error
}

// Value is an instance, it is equivalent to one value.
type Value interface {
	IsNil() bool
	IsBool() bool
	IsInt() bool
	IsDouble() bool
	IsString() bool
	Set(val interface{}) error
	GetBool() (bool, error)
	GetInt() (int64, error)
	GetDouble() (float64, error)
	GetString() (string, error)
}

const (
	MAP_ERROR_WITHOUT_EXIST uint32 = 0x1
	MAP_ERROR_WITH_EXIST    uint32 = 0x2
)

type Map interface {
	Get(key interface{}) (Value, error)
	Set(key, val interface{}, opt uint32) error
}

const (
	PUBSUB_2D_RAISE_NO_ONE_RECV uint32 = 0x1
)

type Pubsub2D interface {
	Publish(name string, x, y, r float64, val interface{}, opt uint32) error
	On(name string, cb func(Value))
	Off(name string)
}
