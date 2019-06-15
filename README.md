# colonio : Library let it easy to use distributed systems

By using the distributed algorithms well, you can take away the load of the servers and create applications with high real-time performance and so on.
But there is little knowledge of distributed algorithms and we can not try it easily.
The purpose of the majority of engineers is to realize their own service, and the use of algorithms is a means.
It is interesting but not essence of work to think and implement difficult algorithms.
The purpose of vein is to make it more versatile and to make it easy for everyone to use distributed algorithms that are easy to use.

## System abstract

The applications that using vein was launched, are connected to each other to create a communication network. 
In that network, there is a ***seed*** (server) for sign-in and distributing setting information, but signaling and data sharing etc. are realized through ***nodes*** (client) without using a server.

![abstract01](md/abstract01.png)

The network is based on WebRTC and can be used even if there are mixed platforms (web / native application, JavaScript / C / C ++ / Python).

![abstract02](md/abstract02.png)

The network is separated by application id, and data is not shared with another applications.

![abstract03](md/abstract03.png)

## Available argorithms and features

### Key value store (KVS)

KVS is a data structure commonly known as a dictionary or hash, and keeps data distributed on nodes. KVS is a mechanism suitable for sharing data to be rewritten between nodes.

### Publish-Subscribe (Pub/Sub) at 2 Dimentional map

Pub / Sub is one type of messaging model. When the Publisher sends a message, the message is propagated to the Subscriber. In Pub / Sub 2D map, it is easy to use a function such as sending a message to Subscriber within a certain range from Publisher. 
The position of the node can be used two-dimensional coordinates such as actual position (latitude, longitude), game map or torus tube.

## License

Apache License 2.0
