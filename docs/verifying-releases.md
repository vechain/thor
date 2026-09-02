# Verifying release binaries

Starting with **v2.5.0**, every Thor release archive contains the binary and a
detached OpenPGP signature made by the VeChain Thor release key. Verifying it
proves the binary was produced by this project's release pipeline and has not
been modified in transit.

Releases before v2.5.0 carry no signature and cannot be verified this way.

## Signing key

```
Fingerprint:  7812 10E3 2A25 054E D725  F325 7E97 E08C 3084 B57E
User ID:      VeChain Thor Release <support@vechain.org>
```

The key is published in two places, either of which will do:

- this repository, at [`docs/release-signing-key.asc`](./release-signing-key.asc)
- [keys.openpgp.org](https://keys.openpgp.org/search?q=781210E32A25054ED725F3257E97E08C3084B57E)

> The public key is deliberately **not** shipped inside the release archives. A
> key distributed alongside the artifact it signs proves nothing: anyone able to
> replace the archive could replace the key with it. Always obtain the key from
> this repository or another channel you already trust.

## Step 1 — import the key (once)

From a keyserver:

```bash
gpg --keyserver keys.openpgp.org --recv-keys 781210E32A25054ED725F3257E97E08C3084B57E
```

Or from this repository, if a keyserver is unreachable:

```bash
curl -fsSLO https://raw.githubusercontent.com/vechain/thor/master/docs/release-signing-key.asc
gpg --import release-signing-key.asc
```

It does not matter which you use, and neither channel needs to be trusted — step 2
establishes that you got the right key.

## Step 2 — confirm you imported the right key

Look the key up by the fingerprint published above:

```bash
gpg --fingerprint 781210E32A25054ED725F3257E97E08C3084B57E
```

```
pub   rsa4096 2026-09-02 [C]
      7812 10E3 2A25 054E D725  F325 7E97 E08C 3084 B57E
uid           [ unknown] VeChain Thor Release <support@vechain.org>
sub   rsa4096 2026-09-02 [S] [expires: 2027-09-02]
      AFFD 441D 59DE 6EDE 25CB  AC2F 6198 953D 5A50 5E23
```

**The command itself is the check.** It fails with `error reading key: No public
key` unless the key you imported really has that fingerprint, so there is nothing
to compare by eye. If it fails, stop — you do not have the right key.

The `[ unknown ]` next to the User ID is normal; it reports that you have not
assigned local trust to the key, not that anything is wrong.

## Step 3 — verify a release

Download the archive for your platform from the
[releases page](https://github.com/vechain/thor/releases) and extract it. Each
archive contains the binary and its `.asc` signature:

```bash
tar xzf thor-linux-amd64.tar.gz          # or: unzip thor-windows-amd64.zip
gpg --verify thor-linux-amd64.asc thor-linux-amd64
```

A successful verification prints:

```
gpg: Signature made Wed 02 Sep 2026 11:04:28 AM UTC
gpg:                using RSA key AFFD441D59DE6EDE25CBAC2F6198953D5A505E23
gpg: Good signature from "VeChain Thor Release <support@vechain.org>" [unknown]
gpg: WARNING: This key is not certified with a trusted signature!
gpg:          There is no indication that the signature belongs to the owner.
```

`Good signature` is the verdict, and the exit status is `0`.

**The `WARNING` does not mean the check failed.** It reports that you have not
assigned local trust to the key in your own keyring — a statement about your
GnuPG configuration, not about the signature. To silence it, see
[Optional](#optional-silence-the-trust-warning).

Depending on your GnuPG version, the output may also list the key fingerprints
after the warning, with or without labels. You do not need to read them: step 2
already established which key you hold.

## Scripting

`gpg` sets a meaningful exit status, so scripts should test that rather than
parse the output:

```bash
if gpg --verify thor-linux-amd64.asc thor-linux-amd64 >/dev/null 2>&1; then
    echo "verified"
else
    echo "VERIFICATION FAILED" >&2
    exit 1
fi
```

| Exit status | Meaning |
|---|---|
| `0` | Good signature |
| `1` | Bad signature — the binary does not match |
| `2` | Could not check — usually the key is missing |

To make the check fail closed on a machine that has never imported the key,
assert the key is present first:

```bash
gpg --list-keys 781210E32A25054ED725F3257E97E08C3084B57E >/dev/null 2>&1 \
    || { echo "signing key not imported" >&2; exit 1; }
```

## Optional: silence the trust warning

If you have confirmed the fingerprint and want `gpg` to stop warning:

```bash
gpg --lsign-key 781210E32A25054ED725F3257E97E08C3084B57E
```

This records a local, non-exportable certification. It changes nothing about the
signature's validity — only how your keyring reports it.

## If verification fails

| Output | Meaning |
|---|---|
| `Can't check signature: No public key` | The signing key is not in your keyring. Do step 1. |
| `BAD signature` | The binary does not match the signature. **Do not run it.** Re-download; if it persists, report it. |
| `error reading key: No public key` at step 2 | The key you imported is not the published one. Do not proceed. |
| Key shows as `expired` | The signing subkey has expired. Signatures made before expiry stay valid; run `gpg --refresh-keys` to pick up the current subkey. |
| Key shows as `revoked` | Do not trust artifacts signed by this key. Check the releases page for an announcement. |

## Reporting

Suspected tampering or key compromise: see [SECURITY.md](./SECURITY.md).
