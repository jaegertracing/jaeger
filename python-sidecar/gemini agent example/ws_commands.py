import websockets


async def ws_to_client_writer(websocket, client_writer):
    try:
        async for message in websocket:
            if isinstance(message, str):
                message = message.encode("utf-8")
            client_writer.write(message)
            if not message.endswith(b"\n"):
                client_writer.write(b"\n")
            await client_writer.drain()
    except websockets.exceptions.ConnectionClosed:
        pass
    except Exception as e:
        print(f"Error in ws_to_client reads: {e}")
    finally:
        client_writer.close()


async def client_reader_to_ws(websocket, client_reader):
    try:
        while True:
            line = await client_reader.readline()
            if not line:
                break
            await websocket.send(line.decode("utf-8"))
    except websockets.exceptions.ConnectionClosed:
        pass
    except Exception as e:
        print(f"Error in client_to_ws writes: {e}")
