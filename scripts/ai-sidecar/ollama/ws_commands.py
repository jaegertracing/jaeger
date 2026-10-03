# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
from typing import Any

import websockets


logger = logging.getLogger(__name__)


async def ws_to_client_writer(websocket: Any, client_writer: asyncio.StreamWriter) -> None:
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
        logger.exception("Error in ws_to_client reads: %s", e)
    finally:
        client_writer.close()
        await client_writer.wait_closed()


async def client_reader_to_ws(websocket: Any, client_reader: asyncio.StreamReader) -> None:
    try:
        while True:
            line = await client_reader.readline()
            if not line:
                break
            await websocket.send(line.decode("utf-8", errors="replace"))
    except websockets.exceptions.ConnectionClosed:
        pass
    except Exception as e:
        logger.exception("Error in client_to_ws writes: %s", e)
