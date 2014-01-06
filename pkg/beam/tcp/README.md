
This is a placeholder for a TCP transport for beam. Since beam is designed to be compatible with SPDY/HTTP2,
most of the heavy lifting can be passed directly to a standard spdy/http2 implementation.

Of course you can't pass file descriptors over tcp, so there is a performance penalty compared to the unix transport.
