go build -gcflags="all=-N -l" -o forgedb .
dlv --listen=:2345 --headless=true --api-version=2 \
    --accept-multiclient exec ./forgedb -- -operation del
