<?php
// Renders parity fixtures with Greenter. Output: $argv[1]/<name>.xml
require getenv('GREENTER_AUTOLOAD') ?: exit("set GREENTER_AUTOLOAD to a vendor/autoload.php with greenter/lite\n");

use Greenter\Model\Client\Client;
use Greenter\Model\Company\{Company, Address};
use Greenter\Model\Sale\{Invoice, Note, SaleDetail, Legend, Detraction, Cuota, Charge, Document, Prepayment, DetailAttribute};
use Greenter\Model\Sale\FormaPagos\{FormaPagoContado, FormaPagoCredito};
use Greenter\Xml\Builder\{InvoiceBuilder, NoteBuilder};

$out = $argv[1];
$tz = new DateTimeZone('America/Lima');
$at = fn(string $s) => new DateTime($s, $tz);

function company(): Company {
    return (new Company())->setRuc('20000000001')->setRazonSocial('EMPRESA DE PRUEBA S.A.C.')->setNombreComercial('PRUEBA')
        ->setAddress((new Address())->setUbigueo('150101')->setDepartamento('LIMA')->setProvincia('LIMA')->setDistrito('LIMA')
            ->setUrbanizacion('URB. CENTRO')->setDireccion('AV. LOS OLIVOS 123')->setCodigoPais('PE')->setCodLocal('0000'))
        ->setEmail('ventas@prueba.pe')->setTelephone('01-5551234');
}
function clientRuc(): Client {
    return (new Client())->setTipoDoc('6')->setNumDoc('20123456789')->setRznSocial('CLIENTE & ASOCIADOS S.A.')
        ->setAddress((new Address())->setUbigueo('150122')->setDireccion('CALLE LAS FLORES 456')->setCodigoPais('PE'));
}

// --- Factura with most optional parts ---------------------------------------
$inv = (new Invoice())->setUblVersion('2.1')->setTipoOperacion('1001')->setTipoDoc('01')->setSerie('F001')->setCorrelativo('123')
    ->setFechaEmision($at('2026-10-27 13:05:00'))->setFecVencimiento($at('2026-11-26 00:00:00'))
    ->setFormaPago(new FormaPagoCredito(1100.00))
    ->setCuotas([(new Cuota())->setMonto(600)->setFechaPago($at('2026-11-10')), (new Cuota())->setMonto(500)->setFechaPago($at('2026-11-26'))])
    ->setTipoMoneda('PEN')->setCompany(company())->setClient(clientRuc())
    ->setSeller((new Client())->setTipoDoc('6')->setNumDoc('20999999999')->setRznSocial('VENDEDOR S.A.C.'))
    ->setDireccionEntrega((new Address())->setUbigueo('150101')->setDepartamento('LIMA')->setProvincia('LIMA')->setDistrito('LIMA')->setDireccion('JR. ENTREGA 1')->setCodigoPais('PE'))
    ->setCompra('OC-2026-55')
    ->setGuias([(new Document())->setTipoDoc('09')->setNroDoc('T001-45')])
    ->setRelDocs([(new Document())->setTipoDoc('99')->setNroDoc('CONTRATO-9')])
    ->setDetraccion((new Detraction())->setCodMedioPago('001')->setCtaBanco('00-123-456789')->setCodBienDetraccion('037')->setPercent(12)->setMount(156))
    ->setDescuentos([(new Charge())->setCodTipo('03')->setFactor(0.01)->setMonto(13)->setMontoBase(1300)])
    ->setSumOtrosDescuentos(13)
    ->setMtoOperGravadas(1000)->setMtoOperExoneradas(100)->setMtoOperGratuitas(50)->setMtoIGVGratuitas(0)
    ->setMtoIGV(180)->setMtoBaseIsc(500)->setMtoISC(50)->setIcbper(0.5)
    ->setTotalImpuestos(230.5)->setValorVenta(1100)->setSubTotal(1330.5)->setRedondeo(0.5)->setMtoImpVenta(1318)
    ->setObservacion('ENTREGA EN ALMACEN <2>')
    ->setLegends([(new Legend())->setCode('1000')->setValue('MIL TRESCIENTOS DIECIOCHO CON 00/100 SOLES'), (new Legend())->setCode('2006')->setValue('Operación sujeta a detracción')])
    ->setDetails([
        (new SaleDetail())->setCodProducto('P001')->setUnidad('NIU')->setCantidad(5)->setDescripcion('PRODUCTO <UNO> & CIA')
            ->setCodProdSunat('43211507')->setCodProdGS1('7750000000017')
            ->setMtoValorUnitario(100)->setDescuentos([(new Charge())->setCodTipo('00')->setFactor(0.1)->setMonto(50)->setMontoBase(500)])
            ->setMtoBaseIsc(500)->setPorcentajeIsc(10)->setIsc(50)->setTipSisIsc('01')
            ->setMtoBaseIgv(500)->setPorcentajeIgv(18)->setIgv(90)->setTipAfeIgv('10')->setTotalImpuestos(140)
            ->setMtoPrecioUnitario(118)->setMtoValorVenta(450)
            ->setAtributos([(new DetailAttribute())->setCode('7000')->setName('Placa')->setValue('ABC-123')]),
        (new SaleDetail())->setCodProducto('BOLSA')->setUnidad('NIU')->setCantidad(2.5)->setDescripcion('BOLSA PLASTICA')
            ->setMtoValorUnitario(200)->setMtoBaseIgv(500)->setPorcentajeIgv(18)->setIgv(90)->setTipAfeIgv('10')
            ->setIcbper(0.5)->setFactorIcbper(0.2)->setTotalImpuestos(90.5)->setMtoPrecioUnitario(236.123456789)->setMtoValorVenta(500),
        (new SaleDetail())->setUnidad('ZZ')->setCantidad(1)->setDescripcion('SERVICIO EXONERADO')
            ->setMtoValorUnitario(100)->setMtoBaseIgv(100)->setPorcentajeIgv(0)->setIgv(0)->setTipAfeIgv('20')->setTotalImpuestos(0)
            ->setMtoPrecioUnitario(100)->setMtoValorVenta(100),
        (new SaleDetail())->setUnidad('NIU')->setCantidad(1)->setDescripcion('MUESTRA GRATIS')
            ->setMtoValorUnitario(0)->setMtoValorGratuito(50)->setMtoBaseIgv(50)->setPorcentajeIgv(0)->setIgv(0)->setTipAfeIgv('21')->setTotalImpuestos(0)
            ->setMtoPrecioUnitario(0)->setMtoValorVenta(50),
    ]);
file_put_contents("$out/invoice-full.xml", (new InvoiceBuilder())->build($inv));

// --- Boleta, contado by default (no FormaPago in Greenter for 03) ------------
$bol = (new Invoice())->setUblVersion('2.1')->setTipoOperacion('0101')->setTipoDoc('03')->setSerie('B001')->setCorrelativo('9')
    ->setFechaEmision($at('2026-10-27 09:00:00'))->setTipoMoneda('PEN')->setCompany(company())
    ->setClient((new Client())->setTipoDoc('1')->setNumDoc('12345678')->setRznSocial('JUAN PEREZ'))
    ->setMtoOperGravadas(100)->setMtoIGV(18)->setTotalImpuestos(18)->setValorVenta(100)->setSubTotal(118)->setMtoImpVenta(118)
    ->setLegends([(new Legend())->setCode('1000')->setValue('CIENTO DIECIOCHO CON 00/100 SOLES')])
    ->setDetails([(new SaleDetail())->setCodProducto('P9')->setUnidad('NIU')->setCantidad(1)->setDescripcion('ITEM')
        ->setMtoValorUnitario(100)->setMtoBaseIgv(100)->setPorcentajeIgv(18)->setIgv(18)->setTipAfeIgv('10')->setTotalImpuestos(18)
        ->setMtoPrecioUnitario(118)->setMtoValorVenta(100)]);
file_put_contents("$out/boleta.xml", (new InvoiceBuilder())->build($bol));

// --- Notes -------------------------------------------------------------------
foreach (['07' => ['FC01', '01', 'ANULACION DE LA OPERACION'], '08' => ['FD01', '02', 'AUMENTO EN EL VALOR']] as $tipo => [$serie, $motivo, $des]) {
    $n = (new Note())->setUblVersion('2.1')->setTipoDoc($tipo)->setSerie($serie)->setCorrelativo('4')
        ->setFechaEmision($at('2026-10-28 10:15:00'))->setTipDocAfectado('01')->setNumDocfectado('F001-123')
        ->setCodMotivo($motivo)->setDesMotivo($des)->setTipoMoneda('PEN')->setCompany(company())->setClient(clientRuc())
        ->setGuias([(new Document())->setTipoDoc('09')->setNroDoc('T001-45')])
        ->setMtoOperGravadas(200)->setMtoIGV(36)->setTotalImpuestos(36)->setMtoImpVenta(236)
        ->setLegends([(new Legend())->setCode('1000')->setValue('DOSCIENTOS TREINTA Y SEIS CON 00/100 SOLES')])
        ->setDetails([(new SaleDetail())->setCodProducto('P001')->setUnidad('NIU')->setCantidad(2)->setDescripcion('PRODUCTO')
            ->setMtoValorUnitario(100)->setMtoBaseIgv(200)->setPorcentajeIgv(18)->setIgv(36)->setTipAfeIgv('10')->setTotalImpuestos(36)
            ->setMtoPrecioUnitario(118)->setMtoValorVenta(200)]);
    if ($tipo === '07') {
        $n->setFormaPago(new FormaPagoCredito(236))->setCuotas([(new Cuota())->setMonto(236)->setFechaPago($at('2026-11-28'))]);
    }
    file_put_contents("$out/note-$tipo.xml", (new NoteBuilder())->build($n));
}
echo "ok\n";
