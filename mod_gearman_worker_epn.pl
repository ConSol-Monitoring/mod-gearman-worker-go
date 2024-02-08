#!/usr/bin/perl

package main;

=head1 NAME

mod_gearman_worker_epn.pl

=head1 SYNOPSIS

  Usage: mod_gearman_worker_epn.pl [options] <socket>

=head1 DESCRIPTION

Run perl monitoring plugins with embedded perl interpreter. Usually this script
is started internally by the mod-gearman-worker. It can be started manually
for developing or testing purposes.

=head1 OPTIONS

    -v|--verbose            print additional debug information
    -c|--cache              enable perl cache
    -r|--run                run single plugin for testing purpose
    socket                  path to socket

=head1 USAGE

Start epn server in verbose mode:

    ./mod_gearman_worker_epn.pl epn.socket -v

then send command lines to the socket:

    echo "test.pl arg1 arg2" | nc -U epn.socket


Test single plugin call

    ./mod_gearman_worker_epn.pl -v --run -- ./plugin.pl <plugin args...>

=head1 AUTHOR

2022, Sven Nierlein, <sven@consol.de>

=cut

use warnings;
no warnings 'redefine';
use strict;
use Time::HiRes;
use Cpanel::JSON::XS;
use Socket ();
use IO::Socket;
use IO::Socket::UNIX;
use Pod::Usage;
use POSIX ":sys_wait_h";
use Getopt::Long;
use Text::ParseWords qw(parse_line);

$| = 1;

###########################################################
# parse and check cmd line arguments
Getopt::Long::Configure('no_ignore_case');
Getopt::Long::Configure('bundling');
Getopt::Long::Configure('pass_through');
my $opt ={
    'help'      => 0,
    'verbose'   => 0,
    'use_cache' => 0,
    'run_only'  => 0,
    'socket'    => [],
};
Getopt::Long::GetOptions(
   "h|help"         => \$opt->{'help'},
   "v|verbose"      => sub { $opt->{'verbose'}++ },
   "c|cache"        => sub { $opt->{'use_cache'}++ },
   "r|run"          => sub { $opt->{'run_only'}++ },
   "<>"             => sub { push @{$opt->{'socket'}}, $_[0] },
) || pod2usage( { -verbose => 2, -message => 'error in options', -exit => 3 } );
pod2usage( { -verbose => 2,  -exit => 3 } ) if $opt->{'help'};

my $unixsocket;
END {
    if($unixsocket) {
        unlink($unixsocket);
    }
}

my $main_loop_interval = 5;
our $child_procs = {};

###########################################################
# listen on the socket and run the plugins
sub _server {
    my($opt) = @_;
    my $socketpath = $opt->{'socket'}->[0];
    unlink($socketpath);
    my $server = IO::Socket::UNIX->new(Local  => $socketpath,
                                    Type      => SOCK_STREAM,
                                    Listen    => Socket::SOMAXCONN,
                ) || die "Couldn't open unix socket $socketpath: $@\n";

    printf("**ePN: listening on %s\n", $socketpath) if $opt->{'verbose'};
    $unixsocket = $socketpath;
    local $SIG{CHLD} = 'IGNORE';
    local $SIG{INT}  = sub {
        CORE::exit(0);
    };
    alarm($main_loop_interval);
    while(1) {
        local $SIG{CHLD} = \&_sigchld_handler;
        local $SIG{ALRM} = \&_timeout_handler;
        while(my $client = $server->accept()) {
            alarm(0);
            _handle_connection($client);
            alarm($main_loop_interval);
        }
    }
    close($server);
}

###########################################################
# cleanup exited child process
sub _sigchld_handler {
    while((my $chldpid = waitpid(-1, &WNOHANG)) > 0) {
        printf("**ePN: chld pid %d exited\n", $chldpid) if $opt->{'verbose'} > 1;
        delete $child_procs->{$chldpid};
    }
    $SIG{CHLD} = \&_sigchld_handler;
}

###########################################################
# check if any chld needs to be killed
sub _check_chld_timeouts {
    my $now = time();
    for my $pid (sort keys %{$child_procs}) {
        my $proc = $child_procs->{$pid};
        if($proc->{'end_time'} < $now) {
            printf("**ePN: killing chld pid %d, %ds timeout (%s) reached but still running\n",
                $pid,
                $proc->{'timeout'},
                scalar localtime $proc->{'end_time'},
            ) if $opt->{'verbose'};
            if($proc->{'end_time'} < $now - 10) {
                kill('KILL', $pid);
            } else {
                kill('INT', $pid);
            }
        }
    }
}

###########################################################
# listen on the socket and run the plugins
sub _timeout_handler {
    _check_chld_timeouts();
    if(getppid() == 1) {
        printf("**ePN: exiting, ppid is 1, this means usually our parent worker has gone away.\n") if $opt->{'verbose'};
        _clean_exit();
    }
    alarm($main_loop_interval);
}

###########################################################
# end children process and exit
sub _clean_exit {
    for my $pid (sort keys %{$child_procs}) {
        kill('INT', $pid);
    }
    if($unixsocket) {
        unlink($unixsocket);
        undef $unixsocket;
    }
    sleep(0.5) if scalar keys %{$child_procs} > 0;
    for my $pid (sort keys %{$child_procs}) {
        kill('KILL', $pid);
    }
    CORE::exit(0);
}

###########################################################
# listen on the socket and run the plugins
sub _handle_connection {
    my($client) = @_;
    my $res;
    eval {
        my $req     = <$client>;
        my $request = _parse_request($req);
        die("**ePN: invalid request: ".($req // 'undef')) unless $request->{'bin'};
        $res        = _handle_request($request);
    };
    my $err = $@;
    if($err) {
        printf("**ePN: errored: %s\n", $err) if $opt->{'verbose'};
        _send_answer($client, {
            rc               => 3, # UNKNOWN
            stdout           => $err,
            compile_duration => 0,
            run_duration     => 0,
        });
        return;
    }
    return unless $res; # parent process can handle next request

    _send_answer($client, $res);

    my $forked = delete $res->{'forked'};
    if($forked) {
        undef $unixsocket;
        CORE::exit(0);
    }
}

###########################################################
# handle a single plugin execution
sub _handle_request {
    my($request, $skip_fork) = @_;

    my $t0 = [Time::HiRes::gettimeofday()];
    my($handler, $err)  = Embed::Persistent::eval_file($request, $opt->{'use_cache'});
    my $elapsed_compile = Time::HiRes::tv_interval($t0);

    if($err) {
        return({
            rc               => 3, # UNKNOWN
            stdout           => $err,
            compile_duration => $elapsed_compile,
            run_duration     => 0,
        });
    }

    # fork now after creating the cache, cache needs to remain in the parent
    my $forked = 0;
    if(!$skip_fork) {
        my $pid = fork();
        if($pid == -1) {
            die("**ePN: failed to fork: ".$!);
        }
        if($pid) {
            my $entry = {
                start_time => time(),
                timeout    => $request->{'timeout'} // 60,
                pid        => $pid,
                request    => $request,
            };
            $entry->{'end_time'} = $entry->{'start_time'} + $entry->{'timeout'};
            $child_procs->{$pid} = $entry;
            printf("**ePN: chld pid %d started\n", $pid) if $opt->{'verbose'} > 1;
            return;
        }
        $forked = 1;
    }

    # continue as child process
    my $t1 = [Time::HiRes::gettimeofday()];
    my($rc, $res) = Embed::Persistent::run_package($request, $handler);
    my $elapsed_run = Time::HiRes::tv_interval($t1);

    return({
        rc               => $rc,
        stdout           => $res,
        compile_duration => $elapsed_compile,
        run_duration     => $elapsed_run,
        forked           => $forked,
    });
}

###########################################################
# parse text or json request into request object
sub _parse_request {
    my($text) = @_;

    chomp($text);
    $text =~ s/\s+$//;
    printf("**ePN: request: %s\n", $text) if $opt->{'verbose'} > 1;

    # json request
    if($text =~ m/^\s*{/mx) {
        return(_request(Cpanel::JSON::XS::decode_json($text)));
    } else {
        my @line = parse_line('\s+', 0, $text);
        my $bin  = shift @line;
        return(_request({
            bin  => $bin,
            args => \@line,
        }));
    }
}

###########################################################
sub _request {
    my($req) = @_;
    if(ref $req ne "HASH") {
        die("expected hash, got: ".(ref $req));
    }
    $req->{'env'}     = $req->{'env'}     // {};
    $req->{'args'}    = $req->{'args'}    // [];
    $req->{'timeout'} = $req->{'timeout'} // 60;
    return($req);
}

###########################################################
sub _send_answer {
    my($client, $res) = @_;
    $res->{'cpu_user'} = POSIX::clock() / 1e6; # value is in microseconds
    $res->{'rc'}       = int($res->{'rc'});
    my $json = Cpanel::JSON::XS->new->utf8->canonical;
    $res = $json->encode($res);
    print $client $res,"\n";
    close($client);
    printf("**ePN: done: %s\n", $res) if $opt->{'verbose'} > 1;
    return;
}

###########################################################
sub _test_run {
    my($args) = @_;
    if($args->[0] && $args->[0] eq '--') {
        shift @{$args};
    }
    printf("**ePN: test run: ".join(" ", @{$args})."\n") if $opt->{'verbose'};

    my $res;
    eval {
        my $req     = join(" ", @{$args});
        my $request = _parse_request($req);
        die("**ePN: invalid request: ".($req // 'undef')) unless $request->{'bin'};
        $res        = _handle_request($request, 1);
    };
    my $err = $@;
    if($err) {
        printf("**ePN: errored: %s\n", $err);
        return(3);
    }

    print $res->{'stdout'};

    printf("**ePN: compile: %.5fs\n", $res->{'compile_duration'}) if $opt->{'verbose'};
    printf("**ePN: runtime: %.5fs\n", $res->{'run_duration'}) if $opt->{'verbose'};
    printf("**ePN: exit:    %d\n", $res->{'rc'}) if $opt->{'verbose'};
    return($res->{'rc'});
}

###########################################################
# one shot mode?
if($opt->{'run_only'}) {
    CORE::exit(_test_run(\@ARGV));
}

###########################################################
pod2usage( { -verbose => 2, -message => 'error in options', -exit => 3 } ) if scalar @{$opt->{'socket'}} != 1;
use subs 'CORE::GLOBAL::exit';
sub CORE::GLOBAL::exit { die sprintf("ExitTrap: %d (Redefine exit to trap plugin exit with eval BLOCK)", $_[0]//0) }
_server($opt);
CORE::exit(0);

################################################################################
package Embed::Persistent;

use strict;

my $plugin_cache = {};

# Offsets in $plugin_cache->{$filename}
use constant MTIME        => 0;
use constant PLUGIN_ERROR => 1;
use constant PLUGIN_HNDLR => 2;

###########################################################
sub valid_package_name {
    local $_ = shift;
    s|([^A-Za-z0-9\/])|sprintf("_%2x",unpack("C",$1))|eg;

    # second pass only for words starting with a digit
    s|/(\d)|sprintf("/_%2x",unpack("C",$1))|eg;

    # Dress it up as a real package name
    s|/|::|g;
    return /^::/ ? "Embed$_" : "Embed::$_";
}

###########################################################
sub eval_file {
    my($request, $use_cache) = @_;

    my $filename = $request->{'bin'};
    my $mtime = -M $filename;
    if($plugin_cache->{$filename} && $plugin_cache->{$filename}[MTIME]) {
        if($plugin_cache->{$filename}[MTIME] <= $mtime) {
            if($plugin_cache->{$filename}[PLUGIN_ERROR]) {
                # failed previously, return last error
                printf("**ePN: cache hit (compile failed) for: %s\n", $filename) if $opt->{'verbose'} > 2;
                return(undef, sprintf("**ePN: failed to compile %s: %s", $filename, $plugin_cache->{$filename}[PLUGIN_ERROR]));
            } else {
                # cache hit, return compiled plugin reference
                printf("**ePN: cache hit for: %s\n", $filename) if $opt->{'verbose'} > 2;
                return $plugin_cache->{$filename}[PLUGIN_HNDLR];
            }
        } else {
            printf("**ePN: need to recompile %s\n", $filename) if $opt->{'verbose'} > 2;
        }
    }

    my $sub;
    open(my $fh, '<', $filename) || return(undef, sprintf("**ePN: failed to open %s: %s", $filename, $!));
    sysread($fh, $sub, -s $fh);
    close($fh);

    # Wrap the code into a subroutine inside our unique package
    # (using $_ here [to save a lexical] is not a good idea since
    # the definition of the package is visible to any other Perl
    # code that uses [non localised] $_).
    my $package = valid_package_name($filename);
    my $hndlr = <<EOSUB ;
package $package;

sub hndlr {
    \@ARGV = \@_;
    local \$^W = 1;
    \$ENV{NAGIOS_PLUGIN} = '$filename';

# line 0 $filename

$sub

}
EOSUB

    $plugin_cache->{$filename}[MTIME] = $mtime if $use_cache;

    # ensure modified Perl plugins get recached by the epn
    no strict 'refs';
    undef %{ $package . '::' };
    use strict 'refs';

    printf("**ePN: compiling %s\n", $filename) if $opt->{'verbose'} > 2;

    # Compile &$package::hndlr. (will run BEGIN blocks)
    # catch prints and add them to the error output
    my $stdout = tie(*STDOUT, 'OutputTrap');
    my $stderr = tie(*STDERR, 'OutputTrap');
    {
        eval $hndlr;
    };
    my $output = <STDOUT>;
    undef $stdout;
    untie *STDOUT;
    $output .= <STDERR>;
    undef $stderr;
    untie *STDERR;

    # $@ is set for any warning and error.
    # This guarantees that the plugin will not be run.
    my $err = $@;
    if($err) {
        $err =~ s/^ExitTrap:.*?line\s+\d+\.//gmx;
        $plugin_cache->{$filename}[PLUGIN_ERROR] = $err;

        # If the compilation fails, leave nothing behind that may affect subsequent compilations.
        return(undef, sprintf("**ePN: failed to compile %s: %s%s", $filename, $err, $output));
    }
    else {
        $plugin_cache->{$filename}[PLUGIN_ERROR] = '';
    }

    # successfully compiled, return reference
    no strict 'refs';
    return($plugin_cache->{$filename}[PLUGIN_HNDLR] = *{ $package . '::hndlr' }{CODE}, undef);
}

###########################################################
sub run_package {
    my($request, $plugin_hndlr_cr) = @_;

    my $has_exit    = 0;
    my $res         = 3;
    my $filename    = $request->{'bin'};
    my @plugin_args = @{$request->{'args'}};

    my $stdout = tie(*STDOUT, 'OutputTrap');
    my $stderr = tie(*STDERR, 'OutputTrap');
    $0 = $filename.(scalar @plugin_args > 0 ? " " : '').join(" ", @plugin_args);

    local %ENV = (%ENV, %{$request->{'env'}}) if($request->{'env'} && scalar keys %{$request->{'env'}} > 0);

    local $SIG{ALRM} = sub { exit(2); };
    alarm($request->{'timeout'});
    eval {
        local $SIG{TERM} = sub { exit(2); };
        local $SIG{INT}  = sub { exit(2); };
        $plugin_hndlr_cr->(@plugin_args);
    };
    alarm(0);

    my $err = $@;
    if($err) {
        if($err =~ m/^ExitTrap:\s+(-?\d+|)/mx) {
            $has_exit = 1;
            $res      = 0+($1 // 0);
        } else {
            chomp($err);
            printf(STDOUT "**ePN: %s: %s\n", $filename, $err);
        }
        ($@, $_) = ('', ''); # reset global perl variables
    }

    my $plugin_output = <STDOUT>;
    undef $stdout;
    untie *STDOUT;
    my $errors = <STDERR>;
    if($errors) {
        $plugin_output .= sprintf("\n[%s]", $errors);
    }
    undef $stderr;
    untie *STDERR;

    $plugin_output = "**ePN: $filename: plugin did not call exit()\n".$plugin_output if $has_exit == 0;

    return($res, $plugin_output);
}

################################################################################

package OutputTrap;
#
# =head1 NAME
#
# OutputTrap
#
# =head1 DESCRIPTION
#
# Tie STDOUT/STDERR to a scalar and caches values written to it.
#
# =cut
#
sub TIEHANDLE {
    my($class) = @_;
    my $me = '';
    bless \$me, $class;
}

sub PRINT {
    my($self, @args) = @_;
    $$self .= join('', @args);
}

sub PRINTF {
    my($self, $fmt, @args) = @_;
    $$self .= sprintf($fmt, @args);
}

sub READLINE {
    my $self = shift;
    return $$self;
}

sub CLOSE {
    my $self = shift;
    undef $self;
}

sub DESTROY {
    my $self = shift;
    undef $self;
}

################################################################################

1;
