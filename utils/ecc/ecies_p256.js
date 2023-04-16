/**
 * @author JQ
 * @elliptic 6.5.4
 * @ecies AES-CBC
 * @curve P256
 */
import * as elliptic from 'elliptic'

/**
 * @param {int} len
 * @returns Buffer
 */
const randomBytes = async function (len) {
    const bytes = new Uint8Array(len)
    await window.crypto.getRandomValues(bytes)
    return Buffer.from(bytes)
}

/**
 * @param {Buffer} iv
 * @param {Buffer} key
 * @param {Buffer} plaintext
 * @returns Buffer
 */
const aes256CbcEncrypt = async function (iv, key, plaintext) {
    const algorithm = { name: 'AES-CBC', iv: iv }
    const keyObj = await window.crypto.subtle.importKey('raw', key, algorithm, false, ['encrypt'])
    const dataBuffer = new TextEncoder().encode(plaintext)
    const encryptedBuffer = await window.crypto.subtle.encrypt(algorithm, keyObj, dataBuffer)
    const encryptedArray = new Uint8Array(encryptedBuffer)
    const encryptedData = Array.prototype.map.call(encryptedArray, x => ('00' + x.toString(16)).slice(-2)).join('')
    return Buffer.from(encryptedData, 'hex')
}

/**
 * @param {String} data
 * @returns Buffer
 */
const sha512 = async function (data) {
    const hashBuffer = await window.crypto.subtle.digest('SHA-512', Buffer.from(data))
    const hashArray = Array.from(new Uint8Array(hashBuffer))
    const hashValue = hashArray.map(b => b.toString(16).padStart(2, '0')).join('')
    return Buffer.from(hashValue, 'hex')
}

/**
 * @param {Buffer} data
 * @param {Buffer} key
 * @returns Buffer
 */
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

/**
 * @param {EC} curve
 * @param {PublicKey} publicKey
 * @returns Buffer, Buffer
 */
const derivePublic = function (curve, publicKey) {
    const tempPrivate = curve.genKeyPair()
    const tempPublic = tempPrivate.getPublic()
    const ephemPublicKey = Buffer.from(tempPublic.encode('hex', false), 'hex')
    let shared = tempPrivate.derive(publicKey.getPublic()).toString('hex')
    if (shared.length < 64) {
        for (let i = 0; i < 64 - shared.length; i++) {
            shared = '0' + shared
        }
    }
    return { ephemPublicKey, shared }
}

/**
 * @param {Hex} pub
 * @param {Buffer} msg
 * @returns String(base64)
 */
export const encrypt = async function (pub, msg) {
    const EC = elliptic.ec
    const curve = new EC('p256')
    const publicKey = curve.keyFromPublic(pub, 'hex')

    const { ephemPublicKey, shared } = derivePublic(curve, publicKey)

    const sharedHash = await sha512(shared)

    const encryptionKey = Buffer.from(sharedHash.buffer, 0, 32)
    const macKey = Buffer.from(sharedHash.buffer, 32)

    const iv = await randomBytes(16)

    const ciphertext = await aes256CbcEncrypt(iv, encryptionKey, msg)

    const hashData = Buffer.concat([iv, ephemPublicKey, ciphertext])

    const realMac = await hmac256(hashData, macKey)

    const response = Buffer.concat([ephemPublicKey, iv, realMac, ciphertext]).toString('base64')
    return response
}

/**
 * @param {String} data
 * @param {Base64} publicKey
 * @returns String(base64)
 */
export const encryptByBase64Public = async function (publicKey, data) {
    const result = await encrypt(Buffer.from(publicKey, 'base64').toString('hex'), Buffer.from(data))
    return result
}

/**
 * @param {String} data
 * @param {Hex} publicKey
 * @returns String(base64)
 */
export const encryptByHexPublic = async function (publicKey, data) {
    const result = await encrypt(publicKey, Buffer.from(data))
    return result
}
