use super::handlers::handle_command;
use crate::cli::parse_args;
use anyhow::Result;

pub(crate) fn run() -> Result<()> {
    let payload = handle_command(parse_args()?)?;
    println!("{payload}");
    Ok(())
}
