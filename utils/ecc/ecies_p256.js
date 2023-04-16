/**
 * @ecies AES-CBC
 * @curve P256
 * @elliptic 6.5.4
 */
import * as elliptic from 'elliptic'

const randomBytes = async function (len) {
    const bytes = new Uint8Array(len)
    await window.crypto.getRandomValues(bytes)
    return Buffer.from(bytes)
}

const aes256CbcEncrypt = async function (iv, key, plaintext) {
    const algorithm = { name: 'AES-CBC', iv: iv }
    const keyObj = await window.crypto.subtle.importKey('raw', key, algorithm, false, ['encrypt'])
    const dataBuffer = new TextEncoder().encode(plaintext)
    const encryptedBuffer = await window.crypto.subtle.encrypt(algorithm, keyObj, dataBuffer)
    const encryptedArray = new Uint8Array(encryptedBuffer)
    const encryptedData = Array.prototype.map.call(encryptedArray, x => ('00' + x.toString(16)).slice(-2)).join('')
    return Buffer.from(encryptedData, 'hex')
}

const sha512 = async function (data) {
    const hashBuffer = await window.crypto.subtle.digest('SHA-512', Buffer.from(data, 'hex'))
    const hashArray = Array.from(new Uint8Array(hashBuffer))
    const hashValue = hashArray.map(b => b.toString(16).padStart(2, '0')).join('')
    return Buffer.from(hashValue, 'hex')
}

const hmac256 = async function (data, key) {
    const keyImported = await window.crypto.subtle.importKey(
        'raw',
        key,
        { name: 'HMAC', hash: { name: 'SHA-256' } },
        false,
        ['sign']
    )
    const hmacBuffer = await window.crypto.subtle.sign(
        { name: 'HMAC' },
        keyImported,
        data
    )
    const hmacArray = Array.from(new Uint8Array(hmacBuffer))
    const hmacValue = hmacArray.map(b => b.toString(16).padStart(2, '0')).join('')
    return Buffer.from(hmacValue, 'hex')
}

export const EciesEncrypt = async function (pub, msg) {
    const EC = elliptic.ec
    const curve = new EC('p256')
    const publicKey = curve.keyFromPublic(pub, 'hex')

    const tempPrivate = curve.genKeyPair()
    const tempPublic = tempPrivate.getPublic()

    const shared = tempPrivate.derive(publicKey.getPublic()).toString('hex')

    const sharedHash = await sha512(shared)
    const encryptionKey = Buffer.from(sharedHash.buffer, 0, 32)
    const macKey = Buffer.from(sharedHash.buffer, 32)

    const iv = await randomBytes(16)

    const ciphertext = await aes256CbcEncrypt(iv, encryptionKey, msg)

    const ephemPublicKey = Buffer.from(tempPublic.encode('hex', false), 'hex')
    const hashData = Buffer.concat([iv, ephemPublicKey, ciphertext])

    const realMac = await hmac256(hashData, macKey)

    return Buffer.concat([ephemPublicKey, iv, realMac, ciphertext]).toString('base64')
}
