export class LoggerService {
  private createdAt: Date;

  constructor() {
    this.createdAt = new Date();
  }

  private delta(): string {
    return (
      "[" + ((Date.now() - this.createdAt.getTime()) / 1000).toFixed(3) + "]"
    );
  }

  debug(...msgs: unknown[]) {
    console.debug(this.delta(), ...msgs);
  }

  info(...msgs: unknown[]) {
    console.info(this.delta(), ...msgs);
  }

  warn(...msgs: unknown[]) {
    console.warn(this.delta(), ...msgs);
  }

  error(...msgs: unknown[]) {
    console.error(this.delta(), ...msgs);
  }
}
