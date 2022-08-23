[ req ]

default_bits       = 2048

distinguished_name = req_distinguished_name

req_extensions     = SAN

[ req_distinguished_name ]

countryName                 = Country Name (2 letter code)

countryName_default         = CN

stateOrProvinceName         = State or Province Name (full name)

stateOrProvinceName_default = ShangHai

localityName                = Locality Name (eg, city)

localityName_default        = ShangHai

organizationName            = Organization Name (eg, company)

organizationName_default    = China

commonName                  = Common Name (e.g. server FQDN or YOUR name)

commonName_max              = 64

commonName_default          = localhost

[ SAN ]

subjectAltName = DNS:localhost,DNS:*.test.com
