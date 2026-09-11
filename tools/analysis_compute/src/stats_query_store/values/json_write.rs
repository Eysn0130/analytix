use anyhow::Result;
use std::io::Write;

pub(super) fn write_json_string<W: Write>(writer: &mut W, value: &str) -> Result<()> {
    if json_string_needs_escape(value) {
        serde_json::to_writer(writer, value)?;
        return Ok(());
    }
    writer.write_all(b"\"")?;
    writer.write_all(value.as_bytes())?;
    writer.write_all(b"\"")?;
    Ok(())
}

pub(super) fn write_json_f64<W: Write>(writer: &mut W, value: f64) -> Result<()> {
    serde_json::to_writer(writer, &value)?;
    Ok(())
}

fn json_string_needs_escape(value: &str) -> bool {
    value
        .as_bytes()
        .iter()
        .any(|byte| *byte < 0x20 || *byte == b'"' || *byte == b'\\')
}

#[cfg(test)]
mod tests {
    use super::*;

    fn write_string(value: &str) -> String {
        let mut out = Vec::new();
        write_json_string(&mut out, value).expect("write json string");
        String::from_utf8(out).expect("utf8 json")
    }

    #[test]
    fn write_json_string_matches_serde_for_plain_utf8() {
        for value in ["", "817100058141129", "美团", "网上银行", "CNY"] {
            assert_eq!(write_string(value), serde_json::to_string(value).unwrap());
        }
    }

    #[test]
    fn write_json_string_falls_back_for_escaped_text() {
        for value in ["a\"b", "a\\b", "line\nnext", "\u{0008}"] {
            assert_eq!(write_string(value), serde_json::to_string(value).unwrap());
        }
    }
}
