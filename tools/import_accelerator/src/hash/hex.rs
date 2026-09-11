pub(crate) fn to_upper_hex(bytes: &[u8]) -> String {
    to_hex(bytes, b"0123456789ABCDEF")
}

pub(crate) fn to_lower_hex(bytes: &[u8]) -> String {
    to_hex(bytes, b"0123456789abcdef")
}

fn to_hex(bytes: &[u8], alphabet: &[u8; 16]) -> String {
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(alphabet[(byte >> 4) as usize] as char);
        out.push(alphabet[(byte & 0x0f) as usize] as char);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn hex_encoding_supports_upper_and_lower_case() {
        let bytes = [0x00, 0x0f, 0x10, 0xab, 0xff];

        assert_eq!(to_upper_hex(&bytes), "000F10ABFF");
        assert_eq!(to_lower_hex(&bytes), "000f10abff");
    }

    #[test]
    fn hex_encoding_returns_empty_for_empty_input() {
        assert_eq!(to_upper_hex(&[]), "");
        assert_eq!(to_lower_hex(&[]), "");
    }
}
