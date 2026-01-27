import asyncio
import math
import os
import signal
import time
import logging
from typing import Optional

from asyncua import Server, ua


logging.basicConfig(level=logging.INFO)


def _env_int(name: str, default: int) -> int:
    raw = os.getenv(name)
    if raw is None or raw.strip() == "":
        return default
    return int(raw)


def _env_str(name: str, default: str) -> str:
    raw = os.getenv(name)
    if raw is None or raw.strip() == "":
        return default
    return raw


async def main() -> None:
    host = _env_str("OPCUA_HOST", "0.0.0.0")
    port = _env_int("OPCUA_PORT", 4840)
    endpoint = f"opc.tcp://{host}:{port}"

    server = Server()
    await server.init()
    server.set_endpoint(endpoint)
    server.set_server_name(_env_str("OPCUA_SERVER_NAME", "Connector Gateway OPC UA Simulator"))

    # Security: keep it simple for local dev.
    server.set_security_policy([ua.SecurityPolicyType.NoSecurity])

    idx = await server.register_namespace(_env_str("OPCUA_NAMESPACE_URI", "urn:connector-gateway:sim"))

    # Address space
    objects = server.nodes.objects
    demo = await objects.add_object(idx, "Demo")

    # IMPORTANT:
    # Give variables stable *string* NodeIds that match the gateway config
    # (e.g. ns=2;s=Demo.Temperature). If we let asyncua auto-assign, the
    # variables end up with ids like ns=2;s=Temperature, which won't match.
    # NOTE: asyncua infers the stored VariantType from the python value.
    # Because `bool` is a subclass of `int` in python, relying on inference
    # can lead to a stored VariantType of Int64, which will then reject real
    # Boolean writes from other stacks (e.g. gopcua) with BadTypeMismatch.
    # For interoperability, always pass an explicit `varianttype` here.
    temperature = await demo.add_variable(
        ua.NodeId("Demo.Temperature", idx),
        "Temperature",
        20.0,
        varianttype=ua.VariantType.Double,
    )
    pressure = await demo.add_variable(
        ua.NodeId("Demo.Pressure", idx),
        "Pressure",
        1.2,
        varianttype=ua.VariantType.Double,
    )
    status = await demo.add_variable(
        ua.NodeId("Demo.Status", idx),
        "Status",
        "OK",
        varianttype=ua.VariantType.String,
    )
    switch = await demo.add_variable(
        ua.NodeId("Demo.Switch", idx),
        "Switch",
        False,
        varianttype=ua.VariantType.Boolean,
    )

    # Extra node for diagnosing client write compatibility.
    write_test = await demo.add_variable(
        ua.NodeId("Demo.WriteTest", idx),
        "WriteTest",
        False,
        varianttype=ua.VariantType.Boolean,
    )

    # Allow writes from client so you can test gateway write path.
    await temperature.set_writable()
    await pressure.set_writable()
    await status.set_writable()
    await switch.set_writable()

    await write_test.set_writable()

    stop_event: asyncio.Event = asyncio.Event()

    def _request_stop(*_: object) -> None:
        stop_event.set()

    try:
        loop = asyncio.get_running_loop()
        for sig in (signal.SIGINT, signal.SIGTERM):
            try:
                loop.add_signal_handler(sig, _request_stop)
            except NotImplementedError:
                # Windows / restricted environments: signal handlers may be unsupported.
                pass
    except RuntimeError:
        pass

    update_ms = _env_int("OPCUA_UPDATE_MS", 500)
    t0 = time.time()

    async with server:
        print(f"OPC UA simulator listening on {endpoint}")
        print("Nodes:")
        print(f"  {temperature.nodeid.to_string()}")
        print(f"  {pressure.nodeid.to_string()}")
        print(f"  {status.nodeid.to_string()}")
        print(f"  {switch.nodeid.to_string()}")
        print(f"  {write_test.nodeid.to_string()}")

        while not stop_event.is_set():
            t = time.time() - t0

            # If values were written externally, keep them as-is.
            # Only auto-update when AUTO_UPDATE=1.
            auto_update = _env_str("OPCUA_AUTO_UPDATE", "1") not in ("0", "false", "False", "no", "NO")
            if auto_update:
                await temperature.write_value(20.0 + 5.0 * math.sin(t / 3.0), ua.VariantType.Double)
                await pressure.write_value(1.2 + 0.2 * math.sin(t / 5.0), ua.VariantType.Double)

                # Flip status occasionally
                if int(t) % 30 < 15:
                    await status.write_value("OK", ua.VariantType.String)
                else:
                    await status.write_value("WARN", ua.VariantType.String)

            await asyncio.sleep(max(update_ms, 50) / 1000.0)


if __name__ == "__main__":
    asyncio.run(main())
