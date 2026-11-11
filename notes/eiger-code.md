# Eiger client code (ClientLibrary.java, ClientContext.java)

- Context is a set of Dep(key, timestamp/version). Versions are assigned by the server.
- insert (line ~1163): sets column.timestamp = 0 so the accepting datacenter
  assigns the version. Blocking call. After it returns: clearDeps(), then
  addDep(new Dep(key, result.version)).
- batch_mutate (line ~1178): zeroes timestamps, sends async to each server with
  getDeps(), clearDeps(), then waits for every callback and calls
  addDeps(result.deps). Does not return until all responses are in.
- Parallelism exists only inside one call (fan-out to servers), not across calls.
- Result: same one-write-at-a-time limit as COPS. The paper never states this,
  but the code does it. The dependency on a write is built from a
  server-assigned version that only comes back in the response.
- Design catch: the timestamp is used for both identity (dep checks) and
  ordering (last-writer-wins), so a client-made id has to be separate.
