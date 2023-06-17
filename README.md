# Mole

Mole is an open-source Go project that leverages the github.com/songgao/water library to create TUN network interfaces on both the server and client sides. It enables the transfer of layer 3 (IP) network stack packets captured from a device to the other side by encapsulating them into layer 4 (UDP, TCP, etc.) packets. This effectively establishes a local network between the client and server through the UDP transport layer. Mole also includes support for simple authentication, but intentionally does not use any encryption mechanisms.

## Features

- TUN interface creation: Mole facilitates the creation of TUN network interfaces, allowing for the capture and manipulation of layer 3 network packets.
- Layer 3 packet transfer: It enables the transfer of captured network packets between the client and server sides, establishing a virtual network connection.
- UDP transport layer: Mole utilizes the UDP transport layer for transmitting encapsulated network packets between the client and server.
- Simple authentication: Mole supports a basic authentication mechanism to ensure authorized access to the network connection.

## Installation

Ensure that you have Go (version 1.16 or higher) installed on your system. Then, execute the following command:

```shell
go get github.com/msdmazarei/mole
```

## Usage

To use Mole, follow these steps:

1. Start the server-side program by running the following command:

   ```shell
   mole -operation_mode=server
   ```

   You can specify additional parameters as needed. By default, the server will listen on `0.0.0.0` and port `3285` for UDP connections.

2. Start the client-side program by running the following command:

   ```shell
   mole -operation_mode=client -address=server-ip -port=server-port
   ```

   Replace `server-ip` and `server-port` with the IP address and port of the Mole server. You can customize the client-side parameters as required.

3. The server and client will establish a connection through the specified transport layer protocol (UDP by default). Upon successful connection, TUN interfaces will be created on both sides.

4. Once the TUN interface is created, the `-onconnect` script will be executed with the device name as the first argument. You can use this script to perform additional network configuration, such as changing the default route or adding iptables rules for NAT support.

5. To capture layer 3 network packets on the client-side, listen to the TUN interface by running:

   ```shell
   tcpdump -i tun0
   ```

   Replace `tun0` with the appropriate TUN interface name.

6. Any captured packets will be encapsulated and transmitted to the server-side via the transport layer connection. The server will then forward these packets to the appropriate destination.

7. Customize the authentication mechanism by modifying the authentication code within the project to fit your requirements. The default secret value is "secret".

Additional Command Line Parameters:

- `-address`: In server mode, specifies the IP address to listen on. In client mode, it specifies the remote server address to connect. The default value is "0.0.0.0".
- `-onconnect`: Path to the script to execute with the device name once a connection is established.
- `-ondisconnect`: Path to the script to execute with the device name once the connection is disconnected.
- `-operation_mode`: Specifies the operation mode. Valid values are "server" or "client". The default value is "server".
- `-port`: In server mode, specifies the listening port. In client mode, specifies the server port to connect. The default value is 3285.
- `-proto`: Specifies the transport layer protocol. Valid values are

 "udp" and "tcp". The default value is "udp".
- `-secret`: Specifies the secret value between the client and server. The default value is "secret".

## Contributing

Community contributions are welcome! If you have suggestions, bug reports, or feature requests, please open an issue on the [GitHub repository](https://github.com/msdmazarei/mole).

To contribute to the project, fork the repository, make your changes, and submit a pull request. We will review your contribution and merge it if it aligns with the project's goals.

When submitting a pull request, ensure that you thoroughly test your changes and provide any necessary documentation or tests.

## License

Mole is licensed under the [MIT License](https://opensource.org/licenses/MIT). You are free to use, modify, and distribute the code according to the terms of the license.

## Acknowledgements

Mole relies on the `github.com/songgao/water` library. We would like to express our gratitude to the developers and contributors of this project for their valuable work.
