import asyncio
import websockets

from sidecar import handle_websocket


async def main():
    async with websockets.serve(handle_websocket, "localhost", 9000):
        print("Jaeger ACP Sidecar listening on ws://localhost:9000")
        await asyncio.Future()


if __name__ == "__main__":
    asyncio.run(main())
