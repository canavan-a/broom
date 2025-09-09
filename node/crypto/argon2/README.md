# Argon2 library

The go library for argon2 does not expose argon2d. This is because of side channel attack risks. We don't care about side channel attacks, we're not hashing passwords and argon2d is the most asic resistant version of argon.
