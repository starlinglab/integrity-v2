# Encryption

## File

The `encrypt` and `decrypt` commands handle file encryption. Currently, only one kind of file encryption is supported, and is described below.

The file encryption algorithm we are using is called [secretstream](https://doc.libsodium.org/secret-key_cryptography/secretstream) and was invented by the libsodium cryptography library. The underlying primitives include ChaCha20 and Poly1305.

With a working implementation of `secretstream` all you need to know to decrypt our files is that the file begins with the 24 byte header, and each message/chunk is 32768 bytes (plaintext) or 32768+17 bytes (ciphertext, due to 17 byte message header). The only exception is the last chunk in the file might of course be shorter than this size.
