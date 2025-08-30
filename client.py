import socket

# Create a UDP socket
# AF_INET specifies the address family for IPv4
# SOCK_DGRAM specifies the socket type for UDP
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)

# Define the server address and port as a tuple
server_address = ('172.16.0.148', 2333)

try:
    # The message to be sent
    message = b'unlock'

    print(f"Sending message '{message}' to {server_address[0]}:{server_address[1]}...")

    # Send the message to the server
    # sendto sends the message to the specified address
    sent = sock.sendto(message, server_address)

    # Print the number of bytes sent
    print(f"Sent {sent} bytes.")

except Exception as e:
    # Print any errors that occur
    print(f"An error occurred: {e}")

finally:
    # Close the socket to free up resources
    print("Closing socket...")
    sock.close()
    print("Socket closed.")
