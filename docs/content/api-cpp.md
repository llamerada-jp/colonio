
---
title: API for C++
type: docs
---

## namespace colonio {#namespacecolonio}

All the functions of colonio are defined in the colonio namespace.

### enum ErrorCode {#namespacecolonio_1ae017c983f2024827bd0e3c42d4215472}

 Values                         | Descriptions                                
--------------------------------|---------------------------------------------
UNDEFINED            | Undefined error is occurred.
SYSTEM_INCORRECT_DATA_FORMAT            | Incorrect data format detected.
SYSTEM_CONFLICT_WITH_SETTING            | The calling method or setting parameter was inconsistent with the configuration in the seed.
CONNECTION_FAILED            | An error on connection start failure.
CONNECTION_OFFLINE            | The node cannot perform processing because of offline.
PACKET_NO_ONE_RECV            | There was no node receiving the message.
PACKET_TIMEOUT            | An error occurs when timeout.
MESSAGING_HANDLER_NOT_FOUND            | 
KVS_NOT_FOUND            | An error that occur when message sent to a non-existent handler.
KVS_PROHIBIT_OVERWRITE            | 
KVS_COLLISION            | 
SPREAD_NO_ONE_RECEIVE            | 

ErrorCode is assigned each error reason and is used with [Error](#classcolonio_1_1Error) and Exception.

**See also**: [Error](#classcolonio_1_1Error), Exception

## class colonio::Colonio {#classcolonio_1_1Colonio}

Main class of colonio. One instance is equivalent to one node.

### virtual  ~Colonio() {#classcolonio_1_1Colonio_1aca8b0075cfc000365de091305b438650}

Destroy the [Colonio](#classcolonio_1_1Colonio) object.

Run disconnect if necessary.

**See also**: [disconnect()](#classcolonio_1_1Colonio_1a0f2a5511e554479f499ea1afdbe9f4d5)

### void connect(const std::string & url,const std::string & token) {#classcolonio_1_1Colonio_1a6549c39141c2085a18aec075a8608ebc}

Connect to seed and join the cluster.

#### Parameters
* `url` Set the URL of the seed. e.g. "wss://host:1234/path". 
* `token` token does not use. This is an argument for future expansion.

Connect to the seed. Also, if there are already other nodes forming a cluster, join them. This method works synchronously and waits for the connection to the cluster to be established. If this method is successful, it will automatically reconnect until disconnect will be called. If you want to work asynchronously, use a asynchronous connect method instead. This method throws [Error](#classcolonio_1_1Error) when an error occurs. At successful completion, nothing is returned.

**See also**: [Colonio::connect](#classcolonio_1_1Colonio_1a6549c39141c2085a18aec075a8608ebc)(const std::string& url, const std::string& token, std::Exec<void(Colonio&)>&& on_success, std::function<void(Colonio&, const Error&)>&& on_failure)

### void connect(const std::string & url,const std::string & token,std::function< void(Colonio &)> && on_success,std::function< void(Colonio &, const Error &)> && on_failure) {#classcolonio_1_1Colonio_1a2198139556faa7c8536e51c7aa4aa3ec}

Connect to seed and join the cluster asynchronously.

#### Parameters
* `url` Set the URL of the seed. e.g. "wss://host:1234/path". 
* `token` token does not use. This is an argument for future expansion. 
* `on_success` The function will call when success to connect. 
* `on_failure` The function will call when failure to connect with error information.

The main purpose of the function is the same as [connect(const std::string& url, const std::string& token)](#classcolonio_1_1Colonio_1a6549c39141c2085a18aec075a8608ebc). This method works asynchronously, and the method returns immediately. Note that the specified callback function is called in a separate thread for processing.

**See also**: [connect(const std::string& url, const std::string& token)](#classcolonio_1_1Colonio_1a6549c39141c2085a18aec075a8608ebc)

### void disconnect() {#classcolonio_1_1Colonio_1a0f2a5511e554479f499ea1afdbe9f4d5}

Disconnect from the cluster and the seed.

This method works synchronously, it will wait for disconnecting and finish thread to process. This method must not be used in any other callback in colonio because of stopping the thread and releasing resources. Once a disconnected colonio instance is reused, it is not guaranteed to work. This method throws [Error](#classcolonio_1_1Error) when an error occurs. At successful completion, nothing is returned.

### void disconnect(std::function< void(Colonio &)> && on_success,std::function< void(Colonio &, const Error &)> && on_failure) {#classcolonio_1_1Colonio_1a879081f550b13dbbc4fcb83ad9ddc9dd}

### bool is_connected() {#classcolonio_1_1Colonio_1a110db534d0fd6add4e6ece374f88efa5}

### std::string get_local_nid() {#classcolonio_1_1Colonio_1a3346422defb290227b2eec7f17a52f88}

Get the node-id of this node.

The node-id is unique in the cluster. A new ID will be assigned to the node when connect. Return an empty string if node-id isn't assigned.

#### Returns
std::string The node-id of this node.

### std::tuple< double, double > set_position(double x,double y) {#classcolonio_1_1Colonio_1a5ee69d380bfb504513c231bf05cae497}

Sets the current position of the node.

This method is only available if the coordinate system is enabled. The coordinate system depends on the settings in seed. This method works synchronously, but other nodes know the position of this node asynchronously.

#### Parameters
* `x` Horizontal coordinate. 
* `y` Vertical coordinate. 

#### Returns
std::tuple<double, double> The rounded coordinates will be returned to the input coordinates.

### Value messaging_post(const std::string & dst_nid,const std::string & name,const Value & message,uint32_t opt) {#classcolonio_1_1Colonio_1ae0893b7fe6695e4d8562d89b0f842f32}

Post a message for the destination node and wait response message.

This method provides the simple feature like RPC.

#### Parameters
* `dst_nid` Target node's ID. 
* `name` 
* `message` 
* `opt` Options.

### void messaging_post(const std::string & dst_nid,const std::string & name,const Value & message,uint32_t opt,std::function< void(Colonio &, const Value &)> && on_response,std::function< void(Colonio &, const Error &)> && on_failure) {#classcolonio_1_1Colonio_1a6d083ab6f348c5e9e6b6dabaa65d573e}

Post a message for the destination node and get response message asynchronously.

#### Parameters
* `dst_nid` Target node's ID. 
* `name` 
* `message` 
* `opt` Options. 
* `on_response` A callback will be called when get response message. 
* `on_failure` A callback will be called when an error occurred.

### void messaging_set_handler(const std::string & name,std::function< void(Colonio &, const MessagingRequest &, std::shared_ptr< MessagingResponseWriter >)> && handler) {#classcolonio_1_1Colonio_1a14de1a4059f4d2229ee80070e2796ace}

register the handler for

If another handler has already been registered, the old one will be overwritten.

#### Parameters
* `name` A name to identify the handler. 
* `handler` The handler function. null if the received message does not require response.

### void messaging_unset_handler(const std::string & name) {#classcolonio_1_1Colonio_1a3d38595b196ebadaf2b3abb83a078b3f}

Release the function registered in the on_call method.

#### Parameters
* `name` A name to identify the handler.

### std::shared_ptr< std::map< std::string, Value > > kvs_get_local_data() {#classcolonio_1_1Colonio_1a1a56ce7bd6c14cf58360f314973a24da}

### void kvs_get_local_data(std::function< void(Colonio &, std::shared_ptr< std::map< std::string, Value >>)> handler) {#classcolonio_1_1Colonio_1a62d4aeddec73c66113900f770f8b0a0f}

### Value kvs_get(const std::string & key) {#classcolonio_1_1Colonio_1ac500ad7d433281247ff6d7933fa121cd}

### void kvs_get(const std::string & key,std::function< void(Colonio &, const Value &)> && on_success,std::function< void(Colonio &, const Error &)> && on_failure) {#classcolonio_1_1Colonio_1a9f0c00bcb518c781b38267cb543f94e2}

### void kvs_set(const std::string & key,const Value & value,uint32_t opt) {#classcolonio_1_1Colonio_1a73a220d4428ed6a1b415b2e1038c3606}

### void kvs_set(const std::string & key,const Value & value,uint32_t opt,std::function< void(Colonio &)> && on_success,std::function< void(Colonio &, const Error &)> && on_failure) {#classcolonio_1_1Colonio_1ad88dfbdf61466022c03331bbbf1abc18}

### void spread_post(double x,double y,double r,const std::string & name,const Value & message,uint32_t opt) {#classcolonio_1_1Colonio_1a5eb4960b060a9e9c5d868840c6cf5d9b}

### void spread_post(double x,double y,double r,const std::string & name,const Value & message,uint32_t opt,std::function< void(Colonio &)> && on_success,std::function< void(Colonio &, const Error &)> && on_failure) {#classcolonio_1_1Colonio_1a029f2c31f9fdfd7c08a41b68e94728af}

### void spread_set_handler(const std::string & name,std::function< void(Colonio &, const SpreadRequest &)> && handler) {#classcolonio_1_1Colonio_1a2f76fa564003a96df591a2c14b1a5163}

### void spread_unset_handler(const std::string & name) {#classcolonio_1_1Colonio_1a590ed591f00047addd34351ede49d2e1}

### protected  Colonio() {#classcolonio_1_1Colonio_1a0e1ca3757b74c464649f99de2a1a17a0}

Construct a new [Colonio](#classcolonio_1_1Colonio) object.

## class colonio::ColonioConfig {#classcolonio_1_1ColonioConfig}

### bool disable_callback_thread {#classcolonio_1_1ColonioConfig_1acdf91be155dab2dc2051856878233da6}

`disable_callback_thread` is a switch to disable creating new threads for exec callback.

The callback functions are exec on [Colonio](#classcolonio_1_1Colonio)'s main thread. The callback function must not block the thread. And should be lightweight. Otherwise, the main thread will be blocked, and the whole [Colonio](#classcolonio_1_1Colonio) process will be slow or freeze. This option is for developing wrappers for JS, Golang, and others. You should not use this option if you don't know the internal structure of [Colonio](#classcolonio_1_1Colonio).

default: false

### unsigned int max_user_threads {#classcolonio_1_1ColonioConfig_1a90abbe622f854475e0a16ceb6acaced9}

`max_user_threads` describes the maximum number of user threads.

Callback functions of [Colonio](#classcolonio_1_1Colonio) will be run on user threads when `disable_callback_thread` is false. User threads are created as needed up to this variable. The developer should set this value to create enough threads when callback functions depend on the other callback functions' return.

default: 1

### std::function< void(Colonio &colonio, const std::string &json)> logger_func {#classcolonio_1_1ColonioConfig_1a68b5df8be7985c536990b802437bc405}

`logger` is for customizing the the log receiver.

`logger` method call when output log. You should implement this method to customize log. [Colonio](#classcolonio_1_1Colonio) outputs 4 types log, `info`, `warn`, `error`, and `debug`. Default logger output info level log to stdout, others are output to stderr. You can customize it by using this interface. Note that the method could be call by multi threads. You have to implement thread-safe if necessary. Log messages past from colonio are JSON format having keys below.

file : Log output file name. level : The urgency of the log. line : Log output line number. message : Readable log messages. param : The parameters that attached for the log. time : Log output time.

#### Parameters
* `json` JSON formatted log message.

### ColonioConfig() {#classcolonio_1_1ColonioConfig_1aace40897aa181edb0204e5d6c1203b46}

## class colonio::Error {#classcolonio_1_1Error}

```
class colonio::Error
  : public exception
```  

[Error](#classcolonio_1_1Error) information. This is used when the asynchronous method calls a failed callback and is thrown when an error occurs in a synchronous method.

**See also**: [ErrorCode](#namespacecolonio_1ae017c983f2024827bd0e3c42d4215472)

### const bool fatal {#classcolonio_1_1Error_1a6f72fbcc6b21947c27e3c5b2ca9795d4}

True if the error is fatal and the process of colonio can not continue.

### const ErrorCode code {#classcolonio_1_1Error_1ae51c58374a1099c86e26bffb5c39e5e5}

Code to indicate the cause of the error.

### const std::string message {#classcolonio_1_1Error_1a7bb5a2b5e0bca49b784c65a61da3e319}

A detailed message string for display or bug report.

### const unsigned long line {#classcolonio_1_1Error_1a8d2d34586c01a67a5325cfd2ad7da98c}

The line number where the exception was thrown (for debug).

### const std::string file {#classcolonio_1_1Error_1a1ac53af41b4b7c1992c006734be02b05}

The file name where the exception was thrown (for debug).

### Error(bool fatal,ErrorCode code,const std::string & message,int line,const std::string & file) {#classcolonio_1_1Error_1a76f1c52b69434b4a69ac2b012a2166d7}

Construct a new [Error](#classcolonio_1_1Error) object.

#### Parameters
* `fatal` True if the error is fatal and the process of colonio can not continue. 
* `code` Code to indicate the cause of the error. 
* `message` A detailed message string for display or bug report. 
* `line` The line number where the exception was thrown. 
* `file` The file name where the exception was thrown.

### const char * what() const {#classcolonio_1_1Error_1a302097acb230c4a27336f0596e331adb}

Override the standard method to output message.

#### Returns
const char* The message is returned as it is.

## class colonio::MessagingResponseWriter {#classcolonio_1_1MessagingResponseWriter}

### virtual  ~MessagingResponseWriter() {#classcolonio_1_1MessagingResponseWriter_1a5fd7cc14c139cc5d1abb8e298913e2c9}

### void write(const Value &) {#classcolonio_1_1MessagingResponseWriter_1ad1d5c5a9e9ebd354b2a2a3ab881e9011}

## class colonio::Value {#classcolonio_1_1Value}

Values in colonio are represented as instance of [colonio::Value](#classcolonio_1_1Value) class.

### Value() {#classcolonio_1_1Value_1adf1200f3ee94898de386576ed714502c}

### Value(const Value & src) {#classcolonio_1_1Value_1a9d0b1c9122d8c86ab3efd9469bb831bf}

Copy construct a new [Value](#classcolonio_1_1Value) object.

#### Parameters
* `src` The copy source object.

### Value(bool v) {#classcolonio_1_1Value_1a30a3af6e49788ae9cbdf1586f2b8c12a}

Construct a new boolean [Value](#classcolonio_1_1Value) object.

#### Parameters
* `v` The source value of the object.

### Value(int64_t v) {#classcolonio_1_1Value_1a0d51df4620ae66b05423ac0545b8a2ba}

Construct a new integer [Value](#classcolonio_1_1Value) object.

#### Parameters
* `v` The source value of the object.

### Value(double v) {#classcolonio_1_1Value_1aa57f22c50677ee221b05f85013cadf53}

Construct a new float number [Value](#classcolonio_1_1Value) object.

#### Parameters
* `v` The source value of the object.

### Value(const char * v) {#classcolonio_1_1Value_1a49d991d7cc36fabdd3a59ebae4d84615}

Construct a new string [Value](#classcolonio_1_1Value) object.

The char array is converted using the std::string constructor, so up to the first `\0` is used to create a [Value](#classcolonio_1_1Value) object.

#### Parameters
* `v` The source value of the object.

### Value(const std::string & v) {#classcolonio_1_1Value_1ac917a8642b786fdf7d92fa2f7526000e}

Construct a new string [Value](#classcolonio_1_1Value) object.

The data was transferred to other nodes is also processed as is.

#### Parameters
* `v` The source value of the object.

### Value(const void * ptr,std::size_t siz) {#classcolonio_1_1Value_1a8c01986ab7673a9d157faa613b4f4c2e}

Construct a new [Value](#classcolonio_1_1Value) object as a binary type.

#### Parameters
* `ptr` 
* `len` Size of binary data.

### virtual  ~Value() {#classcolonio_1_1Value_1ada8ac5e3c87032d16975d52cd2641814}

Destroy the [Value](#classcolonio_1_1Value) object.

### Value & operator=(const Value & src) {#classcolonio_1_1Value_1a73b88558b3ab55c9555c4ba2f7d35253}

Copy operation.

#### Parameters
* `src` The copy source object. 

#### Returns
[Value](#classcolonio_1_1Value)&

### bool operator<(const Value & b) const {#classcolonio_1_1Value_1a7fd077923b0cf7306254d75269ff30c2}

Implementation of comparison operations for std::map and other algorithms.

This comparison operation should not be used as a semantic order in a user program.

#### Parameters
* `b` The object to be compared. 

#### Returns
true 

#### Returns
false

### template<>  <br/>const T & get() const {#classcolonio_1_1Value_1a19c9f47a17504d2229fd20e56035bb16}

Extract the actual value from the object.

The value is passed as a reference type, which will be changed by the call to [reset](#classcolonio_1_1Value_1a3c10785cff62a86ac6d2402c4172cfce) and [set](#classcolonio_1_1Value_1a11fbce54ff85b5c0fbc9f782cd3dcc33) method. The value may be changed by the implementation of the module, such as a setter of another node. Also, depending on the implementation of the module, the value may be changed by another node's setter, etc. Do not hold the returned values as reference types or pointers.

#### Parameters
* `T` Native type, which corresponds to the value stored by [Value](#classcolonio_1_1Value) object. 

#### Returns
const T& The value stored by [Value](#classcolonio_1_1Value) object.

### const void * get_binary() const {#classcolonio_1_1Value_1a07c10f676e3f92e23e3d0f195dcc1714}

### size_t get_binary_size() const {#classcolonio_1_1Value_1abddde79d481130e4bd61eec3b3b7c920}

### Type get_type() const {#classcolonio_1_1Value_1ab4d35438fcc79461b56163b121019eea}

Get the type stored by [Value](#classcolonio_1_1Value) object.

#### Returns
Type The type stored by [Value](#classcolonio_1_1Value) object.

### void reset() {#classcolonio_1_1Value_1a3c10785cff62a86ac6d2402c4172cfce}

Reset the value stored by [Value](#classcolonio_1_1Value) to null.

### void set(bool v) {#classcolonio_1_1Value_1a11fbce54ff85b5c0fbc9f782cd3dcc33}

Set a new boolean value for [Value](#classcolonio_1_1Value) object.

#### Parameters
* `v` The source value of the object.

### void set(int64_t v) {#classcolonio_1_1Value_1a59dfb7e48832839356c77cc3fc06b9b5}

Set a new integer value for [Value](#classcolonio_1_1Value) object.

#### Parameters
* `v` The source value of the object.

### void set(double v) {#classcolonio_1_1Value_1a4e81ea03ed4e43c2d0f298e40717baf3}

Set a new float number value for [Value](#classcolonio_1_1Value) object.

#### Parameters
* `v` The source value of the object.

### void set(const std::string & v) {#classcolonio_1_1Value_1aab8d7a46c3c4f73450a6c37b387191d8}

Set a new string for [Value](#classcolonio_1_1Value) object.

#### Parameters
* `v` The source value of the object.

### void set(const void * ptr,std::size_t siz) {#classcolonio_1_1Value_1ae73bdeb1f3a172954038a585796141e6}

Set a new binary data for [Value](#classcolonio_1_1Value) object.

#### Parameters
* `ptr` 
* `len`

### enum Type {#classcolonio_1_1Value_1a3906898bf51582a3c6b2c419a2dc5aef}

 Values                         | Descriptions                                
--------------------------------|---------------------------------------------
NULL_T            | null
BOOL_T            | boolean
INT_T            | integer
DOUBLE_T            | float number
STRING_T            | string(UTF8 is expected in C/C++)
BINARY_T            | binary

It represents the type that the [Value](#classcolonio_1_1Value) has.

## struct colonio::MessagingRequest {#structcolonio_1_1MessagingRequest}

### std::string source_nid {#structcolonio_1_1MessagingRequest_1afaf475057d0c30a9417de2199dad36f1}

### Value message {#structcolonio_1_1MessagingRequest_1aa8dadeb24482269702d34c64f09faedd}

### uint32_t options {#structcolonio_1_1MessagingRequest_1a10c38802a16a411c4c2c65edf63c724b}

## struct colonio::SpreadRequest {#structcolonio_1_1SpreadRequest}

### std::string source_nid {#structcolonio_1_1SpreadRequest_1a527289ae516845d0cd4100320f8a0d74}

### Value message {#structcolonio_1_1SpreadRequest_1a2c05232306513d466bde89183341c329}

### uint32_t options {#structcolonio_1_1SpreadRequest_1a633f858c42edb5f2161d77ec4161d7e6}

## namespace colonio::LogLevel {#namespacecolonio_1_1LogLevel}

The levels determine the importance of the message.

### const std::string ERROR("error") {#namespacecolonio_1_1LogLevel_1a5e87510ac193097745a021057b178949}

### const std::string WARN("warn") {#namespacecolonio_1_1LogLevel_1ad55d601e14df81bba5378a2521da1574}

### const std::string INFO("info") {#namespacecolonio_1_1LogLevel_1ac0cfb194cda2243c73f6c55b7359e922}

### const std::string DEBUG("debug") {#namespacecolonio_1_1LogLevel_1a4b528a4f861260737252f56cda427ab2}

Generated by [Moxygen](https://sourcey.com/moxygen)
